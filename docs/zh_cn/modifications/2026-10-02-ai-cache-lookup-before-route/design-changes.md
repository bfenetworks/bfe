# mod_ai_cache 缓存查找前移（命中短路前置 / 懒解析下沉）

## 1. 背景

`mod_ai_cache` 一期（见 `2026-09-24-ai-cache-exact-match/design-changes.md`）把
缓存查找回调注册在 `HandleAfterLocation`，命中后 `BfeHandlerFinish` 短路。
与此同时，二期 `mod_ai_intent` 的意图分类采用**懒解析**：不是独立回调，而是
由路由条件原语 `req_ai_intent_in(...)` 在 `mod_ai_route` 规则求值
（`HandleFoundProduct`）时按需触发。

两者叠加产生浪费：**缓存命中的请求在零上游开销返回之前，已经付了一次意图
分类延迟**（生产 GPU ≤50ms P95，本机 Laya CPU 665–800ms），外加路由表查找与
条件求值。命中率越高浪费越大，而命中请求本不需要任何路由决策。

**目标**：

- 缓存命中路径零意图开销：查找短路发生在路由规则求值之前，懒解析不触发；
- 命中请求同时省掉路由表查找与条件求值；
- 未命中路径行为与现状完全一致；任何故障可回滚至现状行为（回退部署镜像）。

**非目标**：不改缓存键/TTL/SSE 命中模板/响应回写/租户隔离/跳过头等既有语义；
不改 `mod_ai_route` / `mod_ai_intent` 的任何逻辑（对它们是纯收益）。

## 2. 变更总览

| 层级 | 变更点 | 影响文件 |
|---|---|---|
| 模块回调 | `cacheRequestHandler` 注册点从 `HandleAfterLocation` 无条件前移到 `HandleFoundProduct` | `bfe/bfe_modules/mod_ai_cache/mod_ai_cache.go` |
| 模块注册 | 注册序移到 `mod_ai_token_auth` 与 `mod_ai_route` 之间（决定回调执行序） | `bfe/bfe_modules/bfe_modules.go` |
| 规则加载 | 缓存规则 Cond 含 `req_ai_intent_in` 时打 warning（抵消优化且语义怪异） | `bfe/bfe_modules/mod_ai_cache/cache_rule_load.go` |
| 文档同步 | 流水线时序更新（`sys_design`）；configuration 文档核验无需变更（本改动不涉配置项） | `bfe/docs/zh_cn/sys_design/ai_cache.md`、`bfe/docs/zh_cn/sys_design/mod_ai_intent.md` |
| 测试 | 单测（注册点/加载告警）+ 集成测试（新增场景） | `bfe/bfe_modules/mod_ai_cache/*_test.go` |

本改动**不新增配置项**（`conf_mod_ai_cache.go` 与 conf 样例均不变）。
响应阶段（`HandleReadResponse` 回写）、计费协同（`AiCacheHit` 跳过扣减）、
访问日志字段（`ai_cache_status`）均不涉及。

## 3. 现状时序与问题定位

BFE 同一回调点内多个 filter 按**模块注册顺序**执行（`AddFilter` 无优先级）。
现状注册序（`bfe_modules.go`，节选）：

```go
// mod_ai_token_auth
mod_ai_token_auth.NewModuleAITokenAuth(),

// mod_ai_route
// Requirement: after mod_ai_token_auth (needs ClientApiKey)
mod_ai_route.NewModuleAiRoute(),

// mod_ai_cache
// Requirement: after mod_ai_route (only cache requests for a resolved
// product/route); before mod_body_process (a cache hit short-circuits
// the request) and before mod_access_pb3 (AiCacheStatus must be set
// before access logging)
mod_ai_cache.NewModuleAiCache(),
```

时序：

```text
HandleFoundProduct:  mod_ai_token_auth → mod_ai_route（规则求值，
                     req_ai_intent_in 首次求值 → 懒解析触发分类）→ mod_ai_intent（仅注入）
HandleAfterLocation: mod_ai_cache（缓存查找；命中 → BfeHandlerFinish 短路）
```

问题：`mod_ai_route` 执行在前，缓存命中与否都要先付分类延迟 + 路由查找。

## 4. 详细改动

### 4.1 回调注册点前移（无条件生效，不引入行为开关）

`mod_ai_cache.go`（现状 `:235-241`）：

```go
// 现状
// register filter for cache lookup at HandleAfterLocation: the product is
// resolved at this point, and a cache hit short-circuits the request
// before any upstream forwarding.
err = cbs.AddFilter(bfe_module.HandleAfterLocation, m.cacheRequestHandler)
```

改为：

```go
// register filter for cache lookup at HandleFoundProduct, ordered between
// mod_ai_token_auth and mod_ai_route (bfe_modules.go), so a cache hit
// short-circuits before route rule evaluation — and the lazy intent resolve
// triggered by req_ai_intent_in — saving classification latency on the hit
// path. req.Route.Product is resolved before HandleFoundProduct fires, so
// Search(product) is unaffected. A hit also short-circuits before
// mod_body_process, and AiCacheStatus is still set before mod_access_pb3
// logging.
err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.cacheRequestHandler)
```

**为什么不加 `LookupAt` 类的行为开关**：

1. 前移与否的功能行为完全一致（miss/hit/计费/租户隔离/缓存键均不变），
   唯一差异是命中请求的日志字段与"少做的功"——开关不保护任何正确性场景；
2. 回滚路径已足够：BFE 无状态，回滚 = 重新部署上一版本镜像（分钟级），
   灰度靠滚动发布，无需热更 conf 逃生门；
3. `mod_ai_cache.conf` 由控制面（ai-gateway-api / conf-agent）生成下发，
   新增字段需跨仓同步，且存在被重新生成覆盖、行为被意外翻转的风险；
4. 避免双分支代码与双模式测试矩阵长期并存。

### 4.2 模块注册顺序调整（`bfe_modules.go`）

```go
// mod_ai_token_auth
mod_ai_token_auth.NewModuleAITokenAuth(),

// mod_ai_cache
// Requirement: after mod_ai_token_auth (never serve cached content to an
// unauthenticated request); BEFORE mod_ai_route — a cache hit short-circuits
// at HandleFoundProduct so route rule evaluation (and the lazy intent resolve
// triggered by req_ai_intent_in) is skipped entirely. req.Route.Product is
// resolved before HandleFoundProduct fires, so the cache rule table
// Search(product) is unaffected. Init order moves with registration: no Init
// dependency on mod_ai_route/mod_ai_intent (own conf/redis/rule table).
// Still before mod_body_process and mod_access_pb3.
mod_ai_cache.NewModuleAiCache(),

// mod_ai_route
// Requirement: after mod_ai_token_auth (needs ClientApiKey); after
// mod_ai_cache (a cache hit has already finished the request)
mod_ai_route.NewModuleAiRoute(),

// mod_ai_intent（位置不变：懒解析注入模块，无 FoundProduct filter，
// 注册序不敏感——见该模块既有注释）
mod_ai_intent.NewModuleAiIntent(),
```

可行性依据：

1. **product 已就绪**：server 先完成 product 查找再触发 `HandleFoundProduct`
   （`bfe_server/reverseproxy.go:724-737`，FindProduct 错误处理在前、回调获取
   在后）；缓存规则按 product 组织（`cache_rule_table.go:59 Search(product
   string)`，`request.go:62` 传 `req.Route.Product`），前移后取值不受影响；
2. **与路由结果无数据依赖**：缓存规则匹配 = `Search(product)` + 自有
   `Cond.Match(req)`，不读 `AiRouteResult`；
3. **body 可读**：FoundProduct 阶段 body 未消费，`request.go` 现有的
   读 body / 生成缓存键 / 恢复 body 逻辑原样可用（`mod_ai_route` 的 body
   原语同在该回调运行）；
4. **鉴权在前**：注册序保证 `mod_ai_token_auth` 先于缓存查找——**绝不允许**
   缓存查找挪到鉴权之前；
5. **Init 序无副作用**：注册序同时决定 Init 序，`mod_ai_cache.Init`（conf、
   Redis 连接池、规则表、web_monitor handler）与 mod_ai_route / mod_ai_intent
   无任何相互依赖。

> 注：本节取代 `2026-09-24-ai-cache-exact-match/design-changes.md` §3 中
> "查缓存放 `HandleAfterLocation`"的注册位置与理由说明（"after mod_ai_route
> (only cache requests for a resolved product/route)"），该文中其余内容
> （规则模型/缓存键/SSE/计费协同等）仍然有效。

### 4.3 新时序

```text
HandleFoundProduct:  mod_ai_token_auth（鉴权/配额计划绑定）
                     → mod_ai_cache（缓存查找）
                         命中 → BfeHandlerFinish：mod_ai_route 不执行、懒解析不触发
                         未命中 → GoOn：保存 miss 上下文（CtxAiCache）、恢复 body
                     → mod_ai_route（规则求值，意图懒解析照常触发）
                     → mod_ai_intent（仅注入，无回调）
HandleAfterLocation: （缓存查找已不在此；其余模块不受影响）
HandleReadResponse:  mod_ai_cache 回写缓存（不变）
HandleRequestFinish: mod_ai_token_auth 配额结算（命中跳过扣减，不变）
```

### 4.4 缓存规则条件约束（加载期告警）

缓存规则 condition 引用 `req_ai_intent_in(...)` 会在缓存查找阶段触发意图
分类——既抵消本优化，语义也怪异（缓存决策不应依赖意图）。处理：
`cache_rule_load.go` 加载规则时检测 Cond 字符串含 `req_ai_intent_in` → 打
**warning 日志**（不拒绝加载，与 mod_ai_route "运行期不校验问题名"的宽容哲学
一致）。现有配置无此用法，加载期告警足以发现误配。

## 5. 行为变化与兼容性

| 项 | 一期行为 | 新行为 | 兼容性评估 |
|---|---|---|---|
| 命中请求的意图开销 | 已分类（白付） | 零分类、零路由查找 | **核心收益**：命中路径延迟下降（GPU ≤50ms / 本机 CPU 665–800ms）；决策服务实际 QPS 随命中率线性下降 |
| 命中请求的路由结果 | `AiRouteResult` 已生成并落日志 | 无（短路在路由前） | 命中请求访问日志 `ai_route_rule_hits` / `ai_target_model` 为空；下游报表需改用 `ai_cache_status=hit` 判命中 |
| 鉴权 | 命中也鉴权 | 不变（注册序保证） | 不变 |
| 计费/配额 | 命中跳过扣减（`AiCacheHit`） | 不变（`HandleRequestFinish` 照常触发） | 不变 |
| 跳过头 `x-bfe-skip-ai-cache` | 支持 | 不变 | 不变 |
| miss 路径 | 读 body → 查 Redis → 保存上下文 → 恢复 body → GoOn | 完全一致（仅回调点不同） | 不变 |
| 命中响应（流式/非流式） | SSE/JSON 模板构造 | 不变 | 不变 |
| 租户隔离 | 缓存键带 key_id/entity 前缀 | 不变 | 不变 |
| 语义缓存（embedding+vector） | 查找在 AfterLocation | 随查找回调一并前移 | 语义检索延迟前移，命中路径仍净收益；未命中路径 embedding 开销与一期总量相同；fail-open 不变 |
| Redis 故障 | fail-open 放行 | 不变 | 不变 |

**不变量清单**（回归测试锚点）：缓存键生成逻辑、`maxBodyBytes/maxValueBytes`
限制、`cacheKeyStrategy` 三策略、TTL 与 `CacheKeyPrefix`、SSE 解析、
`AiCacheStatus` 取值（hit/miss/skip）、命中响应的 header/body 形态。

**拒绝的备选方案**：① 缓存探针前置 + 原回调保留（查两次 Redis，双倍 RTT）；
② 路由求值延迟到缓存 miss 之后（两模块需共享 condition 求值内部状态）；
③ 缓存查找并入 `mod_ai_route`（破坏模块边界与独立热更）；
④ 命中后补记路由结果保日志口径（路由求值 = 懒解析触发点，恰好抵消收益）。

## 6. 测试

1. **单测**（`make test`）：
   - `Init` 注册的查找回调点为 `HandleFoundProduct`；
   - 缓存规则 Cond 含 `req_ai_intent_in` → 加载期 warning，其余规则正常；
   - 回归：现有 `request.go` / `response.go` / `sse.go` 单测全绿（行为未变）；
2. **集成测试**（真实 BFE + Redis + 决策服务 shim）：
   - 同请求二次发送，首次 miss 走分类、第二次 hit 断言决策服务 shim
     **零新增调用**，访问日志 `ai_cache_status=hit` 且 `ai_intent_*` 为空；
   - 流式命中、跳过头、租户 key 隔离回归；
3. **回归**：`make test` 全绿；`bfe_modules.go` 注释更新；
   本改动不新增配置项，`conf/` 样例无需变更。

## 7. 灰度与回滚

| 阶段 | 动作 | 观察 |
|---|---|---|
| 1 | 新版本先行部署灰度实例（与旧版本并行） | 命中请求决策服务调用量下降；访问日志 `ai_route_rule_hits` 空值率 ≈ 缓存命中率（与报表核对） |
| 2 | 全量滚动更新 | 决策服务 QPS、Redis 负载 |

回滚：重新部署上一版本镜像即恢复一期行为（本改动无热更开关，
灰度验证依赖滚动发布的实例级隔离）。

## 8. 风险与应对

| 风险 | 影响 | 应对 |
|---|---|---|
| 日志口径变化导致下游报表偏差 | 命中率/规则命中统计错配 | §5 明示变化点；上线同步报表侧 SQL；灰度阶段比对两版本实例日志 |
| 缓存规则条件误用 `req_ai_intent_in` | 优化被抵消 + 语义怪异 | 加载期 warning（§4.4）+ 配置评审 |
| 前置后命中请求跳过 route 相关回调的隐含假设 | 未来新模块若把必要逻辑挂在"缓存后、路由前"之间 | `bfe_modules.go` 注释写明顺序约束；新模块接入时按注释对齐 |
| 回滚依赖重新部署（无热更开关） | 前置行为异常时需回退镜像，非秒级 | 变更仅移动回调点、功能路径等价（§5 不变量清单）；灰度实例先行观察，风险可控 |
