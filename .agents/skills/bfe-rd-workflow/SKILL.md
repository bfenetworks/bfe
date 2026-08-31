---
name: bfe-rd-workflow
description: 引导用户在 bfe 代码库中完成一次完整的功能研发流程，包括需求对齐、文档修改、代码实现、集成测试与回归验证。
---

# BFE 代码库研发流程

本 Skill 适用于在 `bfe/` 目录中新增或修改 BFE 功能（例如 AI Gateway 的多 API-Key 会话亲和性、限流、计费、路由、协议支持等）的完整研发流程。

## 触发语

当用户提出以下类型请求时启用本流程：

- “我要实现 xxx 功能”
- “请帮我完成 xxx 的代码与测试”
- “请在 BFE 中增加/修改 xxx”

## 执行原则

1. **分阶段暂停确认**：本流程将研发拆分为多个 Phase。每个 Phase 执行完毕后，必须暂停并等待用户确认“可以继续”后，再进入下一个 Phase。不要在未获得用户确认的情况下自动推进到下一阶段。
2. **Git 提交前确认**：在任何 `git commit`、`git push` 或其他会改变 Git 仓库状态的操作之前，必须先向用户说明变更内容并取得明确同意。禁止自动执行 Git 提交或推送。
3. **Git push 默认目标为 origin**：如果获得用户授权执行 `git push`，默认推送到 `origin` 远程仓库，而不是 `upstream`。除非用户明确指定其他 remote，否则不使用 `upstream`。

## 研发阶段

### Phase 0. 需求澄清与范围界定

1. 让用户明确：
   - 功能目标（一句话描述）
   - 验收标准（必须通过的测试/行为）
   - 影响范围（是否改配置协议、是否改 Redis/状态、是否影响既有请求路径）
2. 判断是否为“非平凡改动”（多文件、有架构选择、用户偏好影响实现）：
   - 是 → 调用 `EnterPlanMode`，先出设计文档/plan 再执行。
   - 否 → 直接进入 Phase 1。

完成本阶段后暂停，等待用户确认后再进入下一阶段。

### Phase 1. 修改 modifications 文档

BFE 侧的改动必须在 `bfe/docs/zh_cn/modifications/` 留下修改说明，便于后续维护与审计。

1. 在 `bfe/docs/zh_cn/modifications/` 下新建日期化目录，例如：
   ```
   bfe/docs/zh_cn/modifications/2026-08-26-ai-key-session-affinity/
   ```
2. 在该目录下创建 `design-changes.md`，包含：
   - 背景与目标
   - 主要改动点（数据结构、模块行为、新增字段）
   - 配置影响
   - 兼容性说明
3. 如已有同主题目录，则更新其中的文档，不要重复创建。

完成本阶段后暂停，等待用户确认后再进入下一阶段。

### Phase 2. 更新 configuration 文档

如果改动涉及配置文件（`cluster_conf.data`、`mod_*.conf`、`bfe.conf` 等），必须同步更新中英文配置文档：

1. 中文：`bfe/docs/zh_cn/configuration/`
2. 英文：`bfe/docs/en_us/configuration/`

更新原则：

- 新增字段要说明类型、默认值、是否必填、示例。
- 修改字段要说明前后行为差异。
- 两个语言目录尽量保持内容一致。

完成本阶段后暂停，等待用户确认后再进入下一阶段。

### Phase 3. 更新 sys_design 文档

如果改动涉及系统设计、协议交互或状态存储，需要更新 `bfe/docs/zh_cn/sys_design/`：

1. 查找现有相关文档（例如 `multi_api_key.md`）。
2. 在文档中新增/修改对应章节，说明：
   - 方案选型
   - 数据流
   - 状态存储（如 Redis key 格式、TTL）
   - 边界情况与优化
3. 如需新增独立文档，直接创建 `.md` 文件，并在相关文档中建立链接。

完成本阶段后暂停，等待用户确认后再进入下一阶段。

### Phase 4. 代码实现

1. 阅读相关源码，确认：
   - 配置结构体（`bfe_config/bfe_cluster_conf/cluster_conf/`）
   - 模块加载与处理逻辑（`bfe_modules/mod_*`）
   - 反向代理与 Key 选择逻辑（`bfe_server/reverseproxy.go` 及相关测试）
2. 做最小改动，优先匹配现有代码风格：
   - 不引入未使用的依赖。
   - 不修改与本次需求无关的逻辑。
3. 关键实现完成后，先跑模块级单元测试：
   ```bash
   cd bfe
   go test ./bfe_server/... ./bfe_modules/<相关模块>/...
   ```

完成本阶段后暂停，等待用户确认后再进入下一阶段。

### Phase 5. 编写集成测试设计文档

BFE 的集成测试设计文档位于 `bfe/tests/integration/测试设计文档/`。在写测试代码之前，先补充对应场景的设计文档，便于后续维护与评审。

#### 5.1 新建 scenario 目录

在 `bfe/tests/integration/测试设计文档/` 下新建 scenario 目录，命名规范：

```
scenario-SC<序号>-<简短中文描述>
```

#### 5.2 编写场景级文档 `场景说明.md`

在该目录下编写 `场景说明.md`，包含：

- 场景背景与目的
- 运行模式
- 涉及的 BFE 配置文件
- 测试 Cluster 与关键配置（AIConf、KeyPolicy、路由表等）
- 测试例列表
- 每个测试例的概要说明
- 公共基础设施
- 与其他场景的依赖关系

#### 5.3 编写测试例级文档 `TC-<序号>-<测试例名称>.md`

**每个测试例都必须编写独立的 TC 文档**，文件命名规范：

```
TC-<序号>-<测试例名称>.md
```

例如：`TC-01-高cache命中计费.md`。

每个 TC 文档必须包含以下字段：

| 章节 | 说明 |
|------|------|
| 目的 | 该测试例验证的核心问题或修复点 |
| 前置条件 | BFE 状态、Redis 余额、mock 后端配置、路由绑定等 |
| 请求构造 | Method、Host、Path、Headers、Body |
| 后端响应 | mock 后端返回的状态码与响应体 |
| 执行步骤 | 从启动环境到断言的详细步骤 |
| 预期结果 | 响应状态、backend 命中次数、Redis 余额变化、日志字段等 |
| 清理 | 停止 BFE、关闭 backend、释放 Redis 等 |

> 注意：如果某个测试例在集成测试中无法直接模拟（例如框架回调重复触发），需在文档中说明该场景由单元测试直接覆盖，集成测试仅做间接验证。

#### 5.4 更新总体说明

更新 `bfe/tests/integration/测试设计文档/测试场景总体说明.md`：

- 在“场景清单”中新增一行；
- 在“场景与测试例对应关系”中新增该 scenario 的 TC 列表。

完成本阶段后暂停，等待用户确认后再进入下一阶段。

### Phase 6. 编写集成测试代码

BFE 的集成测试代码位于 `bfe/tests/integration/implementation/`。

1. 选择最接近的现有 scenario 作为模板（例如需要 Redis 的可参考 `scenario-SC07-ai-rate-limit-redis-key`）。
2. 新建 scenario 目录，命名规范：
   ```
   scenario-SC<序号>-<简短英文描述>
   ```
3. 复制模板后，修改 `testdata/` 中的：
   - `server_data_conf/host_rule.data`：host
   - `server_data_conf/route_rule.data`：cluster
   - `cluster_conf/gslb.data`：cluster 名称
   - `mod_ai_route/ai_route.data`：apikey owner / route binding
   - `mod_ai_token_auth/mod_ai_token_auth.conf`、`mod_ai_rate_limit/mod_ai_rate_limit.conf` 等
4. 编写 `_test.go`，覆盖：
   - 正常路径
   - 功能开关开启/关闭对比
   - 边界场景（单 Key、多 ClientKeyId、BFE 重启后状态保持、错误码/降级）
5. 运行集成测试：
   ```bash
   cd bfe
   go test ./tests/integration/implementation/scenario-SC<xx>-<描述>/... -v
   ```

完成本阶段后暂停，等待用户确认后再进入下一阶段。

### Phase 7. 回归验证

1. 运行本次新增 scenario 的测试。
2. 运行与被改动模块相关的既有 scenario：
   - 多 API-Key 轮换：`scenario-SC02-multi-api-key`
   - Redis 限流：`scenario-SC07-ai-rate-limit-redis-key`
3. 运行相关单元测试：
   ```bash
   go test ./bfe_server/... ./bfe_modules/mod_ai_route/... ./bfe_modules/mod_ai_token_auth/...
   ```
4. 如有失败，优先修复；修复后再次回归，直到全部通过。

完成本阶段后暂停，等待用户确认后再进入下一阶段。

### Phase 8. 收尾与总结

1. 检查是否有注释/文档描述的是旧行为，及时同步更新。
2. 向用户汇报：
   - 改动了哪些文件
   - 新增/修改了哪些测试
   - 验证结果
   - 仍存在的风险或待决策点（如有）

3. **Git 提交前必须人工确认**：如果用户要求或流程需要执行 `git commit`、`git push` 等 Git 操作，必须先向用户清晰说明本次提交内容（包含文件清单与主要变更摘要），并取得明确同意后再执行。禁止在未获授权的情况下自动提交或推送。获得授权后，`git push` 默认推送到 `origin`，不要推送到 `upstream`，除非用户明确指定。

## 常见陷阱

- **cluster 名称不一致**：`route_rule.data`、`gslb.data`、ai_route 中的 cluster 名称必须一致，否则 BFE 启动会报 `no backend conf`。
- **ApikeyRouteTableBindings 遗漏**：新增 client apiKey 时，必须在 `mod_ai_route/ai_route.data` 的 `ApikeyRouteTableBindings` 中为其绑定路由表。
- **Redis 绑定 key 格式**：确认代码中实际使用的 key 格式（如 `bfe:ai:key_affinity:<cluster>:<client_key_id>`）。
- **单 Key 优化**：如果实现了“单 Key 不走 Redis”，需要同时避免读和写，否则集成测试里 `Exists` 会误判。
- **BFE 端口冲突**：集成测试里 `processEnv.StartBFE` 会自动分配端口，不要手动写死。

## 推荐命令速查

```bash
# 模块单元测试
cd bfe
go test ./bfe_server/... ./bfe_modules/mod_ai_route/... ./bfe_modules/mod_ai_token_auth/...

# 单个集成测试场景（详细日志）
go test ./tests/integration/implementation/scenario-SC<xx>-<描述>/... -v

# 相关场景回归
go test ./tests/integration/implementation/scenario-SC02-multi-api-key/...
go test ./tests/integration/implementation/scenario-SC07-ai-rate-limit-redis-key/...
```
