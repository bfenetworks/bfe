# BFE 变更说明：AI 路由降级耗尽后返回主选（首个 attempt）错误

- 一句话变更：转发尝试链全部失败后，客户端收到的错误从**最后一个 attempt** 的响应改为**主选 target（首个 attempt）** 的响应；仅当主选 attempt 未产生任何响应（纯传输错误）时，维持返回末次 attempt 的结果。
- 代码库：`bfe/`，分支 **v1.10.0-dev**。§二 行号为撰写时 HEAD `e93be5e0`（2026-11-15）逐条读码核实。
- 关联：`docs/zh_cn/modifications/2026-11-12-ai-route-targets-as-fallbacks/`（尝试链构成：未选中 targets 并入降级）、`2026-10-21-issue-1440-key-exhausted-no-fallback/`（Key 耗尽降级）。
- 状态：**已实现（2026-11-15），尚未提交/推送**。实施记录与验证结果见 §八。

## 一、背景

AI 路由的转发尝试链为 `[选中 target] + [未选中 targets] + [fallbacks]`。当前全部尝试耗尽后，BFE 把**最后一个 attempt** 的错误响应透传给客户端。

末次错误经常掩盖真实故障：

- 典型场景：主选 target（集群 A）因上游故障返回 503，一路降级到 fallback（集群 C），而 C 未配置该模型返回 400 `model_not_found`——客户端看到的是 C 的 400，排查方向被误导，A 的真实故障只在日志里；
- fallback 是"尽力而为"的备用路径，其错误语义（模型缺失、未开通等）不代表主路径的故障性质。

竞品 Bifrost 的语义（`core/bifrost.go:5987-5990`）：全部 fallback 耗尽后**返回主选错误**并恢复主选的响应头，fallback 各次错误只进日志。本次变更对齐该语义。

## 二、现状核对

| 事实 | 位置 |
|------|------|
| 降级循环：每次失败 attempt 的 `res` 在继续前 `res.Body.Close()` 释放连接 | `bfe_server/reverseproxy.go:1557-1559` |
| 循环正常结束（全部失败）后，直接以循环内最后一个 `res` 继续后续处理（normalize、写响应） | `reverseproxy.go:1568-1576` |
| 末次 attempt 可能无响应（传输错误，`res==nil`），走 `err != nil \|\| res == nil` 的合成错误分支 | `reverseproxy.go:1583+` |
| 成功（2xx/3xx）即停，与错误透出无关 | `reverseproxy.go:1520-1530` |
| 主选 attempt 失败时同样走 `res.Body.Close()`，其响应体默认不可复用 | `reverseproxy.go:1557-1559`（i>0 迭代时关闭的是上一轮的 res） |
| 既有集成断言：SC47 TC-03 明确断言末次 attempt（fc）的错误体透出 | `tests/integration/.../sc47_targets_as_fallbacks_test.go` |

关键实现约束：Go 的 `http.Response.Body` 关闭后不可重读，而连接必须及时释放（尤其错误响应也要归还连接）。因此"保留主选响应到最后再透传"不能直接持有原 `res`，需要在主选失败时**读出响应体字节并记录状态码/关键头**，循环结束后用记录的内容重建响应。

## 三、变更设计

### 3.1 行为规格

| 场景 | 现状 | 改为 |
|------|------|------|
| 主选有响应（status+body），全部尝试失败 | 透传末次 attempt 的响应 | 透传**主选** attempt 的响应（status、body；响应头见 3.3） |
| 主选无响应（纯传输错误，res==nil），全部失败 | 末次 attempt 结果（或其合成错误） | **维持现状**：末次 attempt 结果——主选无任何可透出内容时，末次信息更有价值 |
| 任一 attempt 成功 | 该成功响应 | 不变 |

### 3.2 循环内改动（`reverseproxy.go:1510-1560` 区域）

1. 循环前声明记录变量：`var primaryStatus int; var primaryBody []byte; var primaryHeader http.Header; var primaryHasResp bool`；
2. 第 0 次（主选）attempt 失败且决定继续降级时、关闭其 `res` **之前**：若 `res != nil`，读出全部 body（错误体通常很小；设上限防御，如 1MB，超限截断并记日志）、记录 status 与关键头（`Content-Type`、`Retry-After`），置 `primaryHasResp=true`，然后关闭释放连接；
3. 循环结束后：若 `!primaryHasResp` → 维持现状路径；否则丢弃末次 `res`，用记录的三元组构造新的 `bfe_http.Response` 作为最终 `res` 进入后续处理（`normalizeUpstreamError`、访问日志、`BfeStatusCode` 均自然取主选 status）。

### 3.3 响应头口径

只恢复 `Content-Type` 与 `Retry-After` 两个对客户端有语义的头；**不透传**末次/主选的 `X-Bfe-Gw-Error` 等网关标记以外的上游杂散头（避免 fallback 集群拓扑泄漏，与 `normalizeUpstreamError` 的既有边界一致）。`normalizeUpstreamError` 在新 `res` 上照常运行。

### 3.4 可观测性

- 降级循环结束且启用了主选错误透出时，输出一条 `Info` 日志：`fallback exhausted, returning primary error, primary_cluster[%s] status[%d], last_cluster[%s] status[%d]`——排障时可同时看到主选与末次两侧信息；
- 访问日志：最终 `BfeStatusCode` 变为主选 status（对监控/报表而言，"请求失败在主选"比"失败在某个 fallback"更贴近根因）；
- 不新增 Prometheus 指标（既有 `ReqFallback` 计数不变）。

## 四、改动文件

| 文件 | 改动 |
|------|------|
| `bfe_server/reverseproxy.go` | 降级循环记录主选响应三元组；循环结束按 3.1 规格选择最终 `res`；新增上述日志 |
| `bfe_server/reverseproxy_ai_test.go` | 新增单测：主选有响应时透出主选错误（status+body+Content-Type）、主选纯传输错误时维持末次结果、成功路径不受影响 |
| `tests/integration/implementation/scenario-SC47-targets-as-fallbacks/sc47_targets_as_fallbacks_test.go` | **TC-03 断言翻转**：全链失败后错误体从 `500 from fc` 改为 `500 from ta`（首个 attempt），并在注释中说明语义变化 |
| `tests/integration/测试设计文档/scenario-SC47-未选中targets并入降级链/TC-03-链尾兜底与末次错误.md` | 同步断言变更（文件名将"末次错误"改为"主选错误"） |
| `docs/zh_cn/sys_design/mod_ai_route.md` | §6.3 Fallback 流程第 5 步与行为变更说明更新 |

控制面 `ai-gateway-api` 无改动（纯数据面转发语义）。

## 五、行为变更与兼容

**可感知行为变更：降级全部失败时的客户端错误透出**。示例：主选 A 返回 503、fallback C 返回 400——旧行为客户端收 C 的 400，新行为收 A 的 503。

| 组合 | 行为 |
|------|------|
| 新 BFE + 任意控制面 | 纯数据面行为，直接生效 |
| 新旧 BFE 混部 | 各按各的语义；客户端可见错误可能不一致，发布说明中需显式声明 |

兼容性考虑：

- 只影响"全部尝试失败"的尾部路径；成功路径与降级次数不变；
- 改变对客户端可见的错误语义，需评估存量客户端是否按末次错误码编程（如按 fallback 的 400 做参数纠错）——评估结论写入发布说明；
- 回滚：revert 即恢复，无状态、无迁移。

## 六、验收

| 编号 | 验收项 | 判定 |
|------|------|------|
| 1 | 主选 503、fallback 400（不同错误体） | 客户端收到主选的 503 与错误体，`Content-Type` 正确 |
| 2 | 主选连接失败（无响应）、fallback 400 | 维持现状：收到 fallback 的 400 |
| 3 | 主选 503、fallback 200 | 不变：收到 200 |
| 4 | 多 fallback 逐级失败 | 透出主选错误，日志同时记录主选与末次 cluster/status |
| 5 | SC47 TC-03 翻转后 | 全链失败错误体为 `500 from ta`；SC47 其余 TC 不受影响 |
| 6 | 回归 | SC01/SC08/SC47、`bfe_server` 单测全绿 |

## 七、边界与非目标

| 项 | 说明 |
|----|------|
| 恢复全部上游响应头 | 不做——只恢复 `Content-Type`/`Retry-After`，防拓扑泄漏 |
| 错误分类细化降级触发 | 独立立项（维持状态码白名单） |
| 访问日志记录每次 attempt 明细 | 已有逐次尝试日志；不做字段级扩展 |
| 主选响应体超上限 | 截断到 1MB 并记 `Warn`，继续按截断体透出（错误体超过 1MB 属异常，截断优于失败） |

## 八、实施记录

**实施日期**：2026-11-15。基线 `v1.10.0-dev` @ `e93be5e0`（含"未选中 targets 并入降级链"提交；工作区改动未提交，未推送 fork）。

### 8.1 代码落点

| 文件 | 实施内容 |
|------|----------|
| `bfe_server/reverseproxy.go` | 新增 `aiPrimaryErrorSnapshot` 与 `snapshotPrimaryError()`（主选失败关闭连接前读出 status/body/Content-Type/Retry-After，body 1MB 上限截断 + Warn）、`rebuildPrimaryErrorResponse()`（按快照重建响应，仅恢复客户端语义头，不透传上游杂散头）；降级循环 `i==0` 失败继续时快照主选响应与主选 cluster；循环结束判定（快照存在且最终结果仍失败，含末次无响应场景）→ 关闭末次响应、透出主选响应、`lastCluster` 切回主选（`normalizeUpstreamError` 与响应阶段超时用主选配置），记 Info 日志（主选/末次 cluster+status） |
| `bfe_server/reverseproxy_ai_test.go` | 新增 `TestSnapshotPrimaryErrorRebuildRoundTrip`（快照→重建保真 + 杂散头不透传）、`TestSnapshotPrimaryErrorNoResponse`（nil 响应/无 body → 快照 nil）、`TestSnapshotPrimaryErrorTruncation`（超限截断） |

### 8.2 文档落点

- `docs/zh_cn/sys_design/mod_ai_route.md`：§6.3 Fallback 流程第 5 步改两分支出错透出，行为变更说明扩展为两条；
- `tests/integration/测试设计文档/scenario-SC47-未选中targets并入降级链/`：TC-03 更名"链尾兜底与主选错误透出"并改写预期结果；`场景说明.md` 与 `测试场景总体说明.md` 同步；
- 本变更说明。

### 8.3 集成测试（SC47 TC-03 翻转）

- `sc47_targets_as_fallbacks_test.go` `TestTC03_ChainTailFallback`：断言从"末次 fc 错误体"翻转为"主选（ta 或 tb，按加权随机选中者）错误体、绝不透出链尾 fc + 三集群各命中 N 次"。

### 8.4 验证结果

| 验证项 | 命令 | 结果 |
|--------|------|------|
| 编译 | `go build ./bfe_server/` | 通过 |
| 单测（包级，-count=1） | `go test ./bfe_server/...` 等 4 个 AI 模块包 | 全部通过 |
| 本场景 | `go test ./tests/integration/implementation/scenario-SC47-targets-as-fallbacks/ -v` | TC-01~04 全部通过 |
| 回归（-count=1） | SC01、SC02、SC08、SC13、SC46 | 全部通过 |
| 静态检查 | `go vet`、`gofmt -l` | 通过、无未格式化文件 |

### 8.5 实施过程中的设计修正

- **TC-03 断言修正**：初版断言"错误体恒为 `500 from ta`"不成立——主选是加权随机的，透出的是被选中者（ta 或 tb）的错误。修正为"错误体 ∈ {ta, tb} 且绝不为 fc"，配三集群命中数各等于请求数的精确计数。设计文档预期结果已同步。

### 8.6 遗留事项

1. **未提交、未推送 fork**：需要人工确认后提交（含 sign-off）。
2. 错误分类细化降级触发仍为独立后续立项。
