# BFE 变更说明：AI 路由未选中 targets 并入降级链（targets 互为兜底）

- 一句话变更：构建转发尝试链时，把加权选择**未选中的其余 targets 按权重降序插入 fallbacks 之前**，并对全链按 `(cluster_name, model)` 去重——`targets` 内的集群从"只分流、不互备"变为"互为兜底"，与 Bifrost 默认行为对齐。
- 代码库：`bfe/`，分支 **v1.10.0-dev**。§二 行号为设计时 HEAD `cb99da2d`（2026-11-12）逐条读码核实。
- 关联：`docs/zh_cn/modifications/2026-10-21-issue-1440-key-exhausted-no-fallback/`（Key 耗尽降级）、`2026-10-31-issue-1482-epp-reject-short-circuit/`（EPP 拒绝不降级）。
- 状态：**已实现（2026-11-12），尚未提交/推送**。实施记录与验证结果见 §八。

## 一、背景

AI 路由规则（RouteRule）定义了两个列表：`targets`（加权目标，权重和=100）与 `fallbacks`（降级目标，按序尝试）。现状语义：

- `targets` 是**加权负载均衡**：每请求用 `SelectTarget` 随机选中**一个** target 转发，其余 targets 在同一请求内永远不会被尝试；
- `fallbacks` 是**顺序降级**：选中 target 所在集群整体失败后，依次尝试 fallbacks。

后果：多 targets 配置（如 `[A:50, B:50]`）中 A、B **互不兜底**——A 故障时直接降级到 fallbacks，B 完全闲置；A、B 都故障时全部流量涌向 fallbacks。竞品 Bifrost 的做法（`plugins/governance/main.go:624-651`）是把未选中的加权候选**自动按权重降序转为 fallbacks**，加权集群天然互备。本次变更将这一语义引入 BFE。

本期范围仅含"未选中 targets 并入降级链"一项；另两项优化——耗尽后返回主选错误、按错误分类细化降级触发——另行立项，不在本期范围。

## 二、现状核对

| 事实 | 位置 |
|------|------|
| 加权单选：`rand.Intn(100)` 按权重区间选一个 target | `bfe_server/reverseproxy.go:1292-1306`（`SelectTarget`） |
| 尝试链 = `[选中 target] + [fallbacks]`，其余 targets 丢弃 | `reverseproxy.go:1468-1488` |
| batch/文件透传禁用 fallback、body 不可回放禁用 fallback | `reverseproxy.go:1490-1501` |
| 降级循环：成功（<400）停；末位停；本地限流/EPP 拒绝停 | `reverseproxy.go:1510-1560` |
| 换集群前重置（`RetryTime` 清零、body 回放） | `reverseproxy.go:2528-2549` |
| 降级触发：传输错误、nil 响应、≥500、4xx 白名单 {400,401,402,403,422,429} | `reverseproxy.go:2488-2519`（`shouldTriggerFallback`）、`:2420-2427` |
| 控制面校验：targets 内部、`fallbacks` 内部各自 `(cluster_name, model)` 唯一（422）；**跨组允许重复** | `ai-gateway-api/lib/validate/validate.go:692-696`、`:722-726`；契约见 `00-common.md` §6 |
| 集群内实例重试、Key 惩罚过滤（先于降级的内部重试） | `reverseproxy.go:344-367`、`:2886+` |

关键事实：**控制面契约明确允许 targets↔fallbacks 跨组重复**（"fallback 指向主 target 的同集群固定模型，属合理降级配置"）。例如 `targets=[A/x:100]`、`fallbacks=[A/x, B/y]` 是合法配置。

## 三、变更设计

### 3.1 尝试链构建（`reverseproxy.go:1473-1488` 改造）

现状：

```go
attempts = [selectedTarget] + fallbacks...
```

改为：

```
attempts = [选中 target]
         + [其余 targets 按权重降序，按 (cluster_name, model) 去重]
         + [fallbacks 按配置顺序，按 (cluster_name, model) 去重]
```

构造规则：

1. **选中 target** 恒为链首（`IsFallback=false`）；
2. **未选中 targets** 按 `Weight` 降序排在 fallbacks **之前**——高权重候选优先兜底；显式 fallbacks 是用户声明的"兜底中的兜底"，优先级最低；
3. **全链去重**：以 `(cluster_name, model)` 为 key（model 空串=透传，参与判重），优先级高的保留：选中 target > 其余 targets > fallbacks。去重针对的正是**跨组重复**——`targets=[A/x:100]` + `fallbacks=[A/x, B/y]` 在旧逻辑下 fallback A/x 会被尝试一次（与主选同集群同模型，纯属浪费），新逻辑下去重后链为 `A/x → B/y`；组内重复已被控制面 422 拦截，不构成数据面输入；
4. 去重在每次请求构建 attempts 时进行（map 去重，N+M 规模，纳秒级开销，热路径可忽略）。

### 3.2 不加开关

直接变更行为，与 Bifrost 默认语义一致。理由：

- 新语义是原语义的**严格超集**：只影响"选中 target 失败后"的路径，正常路径（一次成功）行为完全不变；
- 存量单 target 配置（`weight=100`）无其余 targets 可并入，行为不变；
- 避免配置面膨胀与"两套语义并存"的文档、测试、排障成本。

行为变化通过发布说明与 `mod_ai_route.md` 设计文档声明（§五）。

### 3.3 既有保护条件对并入项同等生效

| 条件 | 位置 | 对并入 targets 的效果 |
|------|------|------------------------|
| batch/文件透传禁用 fallback | `:1490-1496` | 整体只留首个 attempt，并入项自然被裁掉 |
| body 不可回放 | `:1497-1500` | 同上 |
| 本地限流 `ErrAiRateLimit` 中止降级 | `:1540-1542` | 任一 attempt 命中即停，不继续兄弟 target |
| EPP 拒绝 `ErrEppRejected` 中止降级 | `:1546-1548` | 同上 |
| 换集群重置 `resetRequestForRetry` | `:2528-2549` | 每次降级尝试前重置，对并入项一致 |

## 四、改动文件

| 文件 | 改动 |
|------|------|
| `bfe_server/reverseproxy.go` | `:1473-1488` 尝试链构建：插入未选中 targets（按权重降序）+ `(cluster_name, model)` 全链去重；`aiForwardAttempt` 可增 `FromTarget bool` 字段区分"target 兜底"与"配置 fallback"（监控/日志用，可选） |
| `bfe_server/reverseproxy_test.go`（或既有 AI 测试文件） | 新增单测：多 targets 降级顺序、跨组重复去重、单 target 行为不变、batch 透传裁剪 |
| `docs/zh_cn/sys_design/mod_ai_route.md` | 降级语义描述更新（"target 加权选择、fallback 顺序降级" → 补充 targets 互备语义） |

控制面 `ai-gateway-api` **无代码改动**（RouteRule 结构、导出、校验均不变：targets 权重与 fallbacks 顺序原样下发，跨组重复契约由数据面去重兜底，控制面无需收紧校验）；`conf/` 无需改动（无新增配置项）。

## 五、行为变更与兼容

**可感知行为变更：多 targets 配置的故障路径**。示例 `targets=[A:50, B:50]`、`fallbacks=[C]`：

| 场景 | 旧行为 | 新行为 |
|------|--------|--------|
| 选中 A，A 故障 | 直接尝试 C | 先尝试 B，B 也失败再尝试 C |
| A、B 都故障 | 全部涌向 C | 请求 1：A→B→C；请求 2：B→A→C |
| 单 target（weight=100） | 不变 | 不变 |
| 正常路径（一次成功） | 不变 | 不变 |

兼容矩阵：

| 组合 | 行为 |
|------|------|
| 新 BFE + 旧控制面 | 纯数据面行为，与报文无关，直接生效 |
| 新旧 BFE 混部 | 各按各的语义，无协议/报文兼容问题 |

回滚：revert 即恢复旧语义，无状态、无迁移。

## 六、验收

| 编号 | 验收项 | 判定 |
|------|--------|------|
| 1 | 单 target 配置 | 行为与旧版完全一致（无多余 attempt） |
| 2 | 多 targets，选中项故障 | 按权重降序尝试其余 targets，再 fallbacks |
| 3 | `targets=[A/x:100]` + `fallbacks=[A/x, B/y]` | 链为 `A/x → B/y`，A/x 不重复尝试 |
| 4 | 全部尝试耗尽 | 返回最后一个 attempt 的错误（"返回主选错误"优化未实施，维持现状） |
| 5 | batch/文件透传 | 仍只尝试一次，禁用降级 |
| 6 | 本地限流 / EPP 拒绝 | 仍中止降级，不进入兄弟 target |
| 7 | 存量回归 | `mod_ai_route`、`bfe_server` 既有单测与集成场景全绿 |

单测命令：

```bash
go test ./bfe_server/... -run 'AiRoute|Fallback|Attempt' -v
go test ./bfe_modules/mod_ai_route/... -v
```

集成场景（`integration-test` 仓库）：新增"target 故障降级到兄弟 target"、"跨组重复只尝试一次"用例；回归 SC02/SC10 等既有 AI 路由场景。

## 七、边界与非目标

| 项 | 说明 |
|----|------|
| 耗尽后返回主选错误 | 另行立项；本期维持"返回末次错误" |
| 按错误分类细化降级触发条件 | 另行立项；本期维持状态码白名单 |
| 开关 / 灰度配置 | 不做，直接变更行为（§3.2） |
| 跨请求 target 熔断（动态权重） | 更大话题，另立项 |
| 控制面校验收紧（禁止跨组重复） | 不做——跨组重复是文档明确的合法配置，数据面去重已兜底 |
| 每次尝试上限 | 不加硬上限；单请求最多尝试 = 1 + (N-1) + M，降级放大面由既有监控观察 |

## 八、实施记录

**实施日期**：2026-11-12。基线 `v1.10.0-dev` @ `cb99da2d`（工作区改动未提交，未推送 fork）。

### 8.1 代码落点

| 文件 | 实施内容 |
|------|----------|
| `bfe_server/reverseproxy.go` | 新增 `buildAiForwardAttempts()`（`:1314` 起）：构建 `[选中 target] + [未选中 targets 按权重降序] + [fallbacks]` 尝试链，`(cluster_name, model)` 全链去重（优先级：选中 > 其余 targets > fallbacks）；`ServeHTTPForAI` 原内联构建逻辑（旧 `:1473-1488`）替换为该函数调用；import 增 `sort` |
| `bfe_server/reverseproxy_ai_test.go` | 新增 `TestBuildAiForwardAttemptsSingleTarget` / `...RemainingTargetsWeightDescending` / `...FallbacksAfterTargets` / `...CrossGroupDedup` / `...FallbackOnlyWhenNoTargets` 5 个单测 |

### 8.2 文档落点

- `docs/zh_cn/sys_design/mod_ai_route.md`：§1.1 背景、§2 术语表 Fallback 定义、§6.1 流程第 6 步、§6.3 Fallback 流程整节重写（含行为变更说明）、§7.4 伪代码注释；顺带修正 §6.3 原描述与代码不符处（4xx 白名单实际触发降级）；
- `tests/integration/测试设计文档/scenario-SC47-未选中targets并入降级链/`：`场景说明.md` + TC-01~04；
- `tests/integration/测试设计文档/测试场景总体说明.md`：场景清单与 TC 对应关系新增 SC47；
- 本变更说明。

### 8.3 集成测试（新增 SC47）

- 设计文档：`tests/integration/测试设计文档/scenario-SC47-未选中targets并入降级链/`（`场景说明.md` + TC-01…TC-04）；
- 实现：`tests/integration/implementation/scenario-SC47-targets-as-fallbacks/`（含 `testdata/` 与 4 个用例），单 `ai_route.data` 以 `req_host_in` Host 条件区分 4 组 targets/fallbacks 规则；
- 公共框架：`tests/integration/common/util.go` 的 `clusterSubName()` 增加 `cluster_ta/tb/fc` 映射（沿用既有 switch 模式，未知名称原落到默认 `"sub"` 导致 gslb 找不到后端）。

### 8.4 验证结果

| 验证项 | 命令 | 结果 |
|--------|------|------|
| 编译 | `go build ./bfe_server/` | 通过 |
| 单测（包级） | `go test ./bfe_server/... ./bfe_modules/mod_ai_route/... ./bfe_modules/mod_ai_token_auth/... ./bfe_modules/mod_ai_rate_limit/...` | 全部通过 |
| 新场景 | `go test ./tests/integration/implementation/scenario-SC47-targets-as-fallbacks/ -v` | TC-01~04 全部通过 |
| 回归 | SC01 路由表查找、SC08 模型 Fallback 重算、SC02 多 API-Key、SC46 Key 惩罚时长 | 全部通过 |
| 静态检查 | `go vet`（改动包）、`gofmt -l` | 通过、无未格式化文件 |

### 8.5 实施过程中的设计修正

1. **TC-01 断言修正**：设计稿"每条请求恰好 2 次上游交互"不成立——直接选中健康 target 的请求只有 1 次交互。改为：fallback 集群零命中（新旧行为决定性区分）+ 健康 target 服务每条请求恰好一次 + 两种选中结果均出现。
2. **`IsFallback` 字段语义**：确认该字段当前仅写入未被读取（预留给监控/日志），未选中 targets 标记为 `false`（来自 targets 列表而非 fallbacks 列表），保持字段语义一致性。

### 8.6 遗留事项

1. **未提交、未推送 fork**：需要人工确认后提交。
2. 耗尽后返回主选错误、错误分类细化降级触发仍为独立后续立项。
