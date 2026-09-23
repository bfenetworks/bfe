# BFE Issue #1387 修复方案

- Issue: https://github.com/bfenetworks/bfe/issues/1387
- 缺陷：API-Key 的 `allow_models` / `block_models` 校验对象是请求体原始模型名，且校验时机在路由/模型重定向生效之前；配置「允许模型 = provider 真实模型（`glm-5.2`）+ 集群模型重定向（`glm-5.2-abc` → `glm-5.2`）」时，客户端发 `model=glm-5.2-abc` 被 400 `MODEL_NOT_ALLOWED` 误拒，模型重定向与 Key 模型访问控制无法组合
- 代码库：`bfe/`（已对照当前代码逐条核实，行号基于 HEAD `fdd4efd7`）
- 关联设计：`docs/zh_cn/modifications/2026-08-21-optimize-ai-model-body-rewrite/`（`computeTargetModel` 与目标模型回写）；`docs/zh_cn/modifications/2026-09-23-issue-1384-gemini-token-auth-model-whitelist/`（ClientModel 的 body-first + 路径兜底提取，本方案继承该链路）
- 文档联动（另仓）：`ai-gateway-web/docs/zh-cn/09-api-key.md:192`（§9.7 运行时生效机制）、`05-ai-business-cluster.md:119`（§5.6 模型重定向）、`10-route.md:158`（§10.4 目标与权重）

## 一、根因

### 根因链（代码级已核实）

1. **校验对象与时机**：`ValidateUserTokenByReq`（`bfe_modules/mod_ai_token_auth/token_rule_table.go:101`）的模型 allow/block 校验块在 `:192-243`：`condition.ReqBodyJsonFetch(req, "model", nil)`（`:193`，#1384 后 `:198` 有 Gemini 路径兜底）取到**原始 client model**，与 `token.Models` / `token.BlockModels` 逐字比对（`:211-242`），不命中即 `CodeModelNotAllowed`。`token.Models` / `token.BlockModels` 即控制面合并 Key 自身 + 挂载 Entity 及全部祖先组织后的有效列表（数据面 `Token` 结构仅这两个字段，`token.go:40-41`）。
2. **校验早于路由**：`tokenFoundProductHandler`（`mod_ai_token_auth.go:327-355`）挂在 `HandleFoundProduct`（`:412`）；模块注册顺序 `bfe_modules/bfe_modules.go:149-154` 为 **token_auth 先于 route**（注释明确要求 route 依赖 token_auth 设置的 `ClientApiKey`）。即校验执行时 `AiRouteResult` 尚未解析，任何「按转发后模型校验」的方案都无法在鉴权期就地完成。
3. **重定向生效点**：模型重定向在转发阶段才应用——`computeTargetModel`（`bfe_server/reverseproxy.go:1478-1501`）顺序为 路由目标 Model 覆盖（`:1482-1484`）→ StripPrefix（`:1487-1491`）→ 集群 `ModelMapping`（`:1494-1498`，`bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:225`）；调用点 `:1559`，写入 `aiMeta.TargetModel` `:1562`，回写请求体 `:1567-1584`。
4. **请求流**：`ServeHTTPForAI` → `HandleFoundProduct` 回调（`reverseproxy.go:1193-1217`，鉴权 400 在此短路）→ `aiResult` 检查（`:1220`）→ 加权选择目标（`:1264-1266`，`SelectTarget` `:1096-1110`）→ 组装主目标 + fallback 尝试列表（`:1268-1283`）→ 转发循环（`:1300-1331`）→ `aiClusterInvoke`（`:1615`）→ `doSingleAIForward`（`:1504`）→ `clusterInvoke`（`:1611`）。

鉴权拒绝在前（步骤 1-2），重定向解析在后（步骤 3-4），请求到不了 `computeTargetModel` 即被 400 → `ai_target_model` 停留在请求模型。研发裁定：允许模型应设 provider 真实模型（转发后），当前拒绝不符合设计意图。

### 与 issue 原文的两处出入（按代码现状修正）

1. **行号**：issue 行号基于 #1384 修复前；现模型校验块为 `token_rule_table.go:192-243`（`:198` 已有 Gemini 路径兜底，但兜底对象仍是原始 client model，与本 issue 无关）。
2. **「无路由命中 target=ClientModel 透传」不成立**：`AiRouteResult` 为 nil 时 `ServeHTTPForAI` 直接 404（`reverseproxy.go:1221-1229`），不存在透传转发。真实行为变化见第五节——改动前此类请求可能先吃鉴权期 400 `MODEL_NOT_ALLOWED`，改动后统一为 404。

## 二、修复步骤

核心思路（与 issue 一致）：**仅把模型 allow/block 检查后移到「目标模型已解析」之后**；Key 识别 + 启用 + 过期 + 配额余额 + 子网保留在 `HandleFoundProduct` 不动（route 表优先级 API-Key > Entity > Global 依赖 `ClientApiKey` 已设置，且不应为无效/过期 Key 做无谓路由）。

### 步骤 1（必改）：`mod_ai_token_auth` 新增目标模型校验方法

新文件 `bfe_modules/mod_ai_token_auth/model_check.go`（或置于 `token_rule_table.go` 内）：

```go
// ValidateTargetModel 用已解析的转发后目标模型校验 Token 的有效允许/禁止
// 模型列表（issue #1387）。仅在目标模型完全解析后调用（路由目标覆盖 +
// StripPrefix + ModelMapping 均已应用）。无 TokenAuthContext（未匹配
// token 规则）或未配置列表时放行——与改动前「无规则不校验」一致。
func (m *ModuleAITokenAuth) ValidateTargetModel(req *bfe_basic.Request, targetModel string) *bfe_basic.AiError
```

实现要点：

- 取 `GetTokenAuthContext(req)`（`mod_ai_token_auth.go:454`）；`ctx == nil || ctx.Token == nil` → 返回 nil。`TokenAuthContext` 由 `SetTokenAuthContext`（`:352`，定义 `:495-517`）在鉴权通过时设置，是本方案无需额外通道的原因；
- `len(tok.Models) == 0 && len(tok.BlockModels) == 0` → 返回 nil（未配置列表的 Key 不校验，与现状一致）；
- block 优先于 allow，循环比对顺序与现 `:211-242` 保持一致；比较前 `strings.TrimSpace`（沿用现 `:210`）；
- **空 targetModel**（所有提取途径都拿不到模型，如非 Gemini 路径且无 body `model`）：保持现行语义，返回 `CodeInvalidRequest` 400，消息沿用 `"Model not found in request body: %v"`——改动前该 400 发生在鉴权期，改动后推迟到转发前，错误码与文案不变；
- 拒绝时：`SetAiAuthInfo(req, CodeModelNotAllowed, nil)`（`token_rule_table.go:93-99`）+ `m.state.ReqAuthFail.Inc(1)`（`:343` 语义平移），返回 `NewAiErrorWithDetails(CodeModelNotAllowed, ...)`，消息文案沿用 `"Model %s not allowed by key %s"` / `"Model %s blocked by key %s"`，`AiErrorDetail.Model` 填 **targetModel**（而非原始模型）。

### 步骤 2（必改）：删除鉴权期的模型校验块

`token_rule_table.go:192-243` 整段 `if len(token.Models) > 0 || len(token.BlockModels) > 0 { ... }` 删除。核实点：

- `ValidateUserTokenByReq` 仅 `mod_ai_token_auth.go:341` 一个调用点，删除安全；
- #1384 的 Gemini 路径兜底随块移除，但其功能已由 `extractClientModel`（`bfe_server/http_conn.go:522-530`，body-first + 路径兜底）在请求入口填充 `aiMeta.ClientModel`，经 `computeTargetModel` 流入 targetModel，语义继承、不回归（SC16 TC-09 链路不变，见第三节）。

### 步骤 3（必改）：`doSingleAIForward` 注入校验（推荐注入点）

`bfe_server/reverseproxy.go`，`:1562` 之后、`:1611` `clusterInvoke` 之前：

```go
aiMeta.TargetModel = targetModel                                     // :1562
// issue #1387: 在目标模型解析后校验 Key 允许/禁止模型列表
if module := srv.Modules.GetModule(mod_ai_token_auth.ModAITokenAuth); module != nil {
    if atm, ok := module.(*mod_ai_token_auth.ModuleAITokenAuth); ok {
        if aiErr := atm.ValidateTargetModel(basicReq, targetModel); aiErr != nil {
            return aiErr.CreateErrorResponse(basicReq), closeAfterReply, nil, bodyModel
        }
    }
}
res, action, err = p.clusterInvoke(srv, cluster, basicReq, rw)       // :1611
```

选此点的核实结论：

- `targetModel` 已是最终转发值（与 `:1567` 回写 body 的值一致），拒绝时 `ai_target_model` 即重定向后模型，满足验收 1/2 的日志断言；
- 模块实例获取沿用既有模式（`srv.Modules.GetModule` + 类型断言，参考 `:1676-1677` 对 `mod_ai_rate_limit` 的用法）；`bfe_server` 已 import `bfe_modules/mod_ai_*`（`:53-54`），且 `mod_ai_token_auth` 不依赖 `bfe_server`，**无 import 环**；
- **key 轮换不重复触发**：校验失败返回本地 400 后，key 轮换循环（`:1690-1770`）的 `default:` 分支（`:1766-1768`「other 4xx client errors: stop key-level retry」）直接返回，不会无谓轮换；
- **fallback 语义正确**：400 在 `aiFallbackStatusCodes` 默认表内（`:1779-1786` 含 400），集群级 fallback 循环（`:1300-1331`）会对下一个 attempt 重新执行 `doSingleAIForward` → 重算 `computeTargetModel` → 复校验——正是验收 5「fallback 目标按各自 target 复校验」；全部 attempt 被拒时，最后一个 attempt 的 400 返回客户端；
- 在发给后端之前拒绝，错误码/错误体保持 `MODEL_NOT_ALLOWED`。

**备选注入点（不推荐，记录备查）**：`mod_ai_token_auth` 注册 `HandleForward` 过滤器（`clusterInvoke` 内 `:380-392` 触发）读 `aiMeta.TargetModel` 校验。模块分离更干净，但 `HandleForward` 按 key 轮换/后端重试重复触发需自行去重；且拒绝点在后端选择之后，白白完成建连。二选一取推荐方案。

### 步骤 4（连带，必改）：fallback 成功后清除 RejectReason

 attempt 1 校验失败会置 `AiAuthInfo.RejectReason = MODEL_NOT_ALLOWED`（access log `ai_auth_reject_reason` 字段，输出点 `bfe_modules/mod_access_pb3/request_log.go:471-472`）；若后续 fallback attempt 成功，该残留会让一条成功请求的日志带拒绝原因。在 `ServeHTTPForAI` 成功 break 分支（`:1310-1313`）将 `aiBasicInfo.AiAuthInfo.RejectReason` 置空。备选（不清理、文档说明字段含义为「请求过程中曾发生的拒绝」）不推荐——成功请求带 reject reason 极易误导排障与监控统计。

### 步骤 5（文档同步，另仓 `ai-gateway-web`）

- `09-api-key.md:192` §9.7：模型访问控制从「路由前（鉴权期）」改为「转发前（目标模型解析后）」，与限流/配额的相对位置重述；
- `05-ai-business-cluster.md:119` §5.6：补充模型重定向与 `allow_models`/`block_models` 的组合语义——**允许/禁止模型填转发后模型名**；
- `10-route.md:158` §10.4：路由目标「指定模型」覆盖参与校验对象计算（目标覆盖 → 裁剪 → 重定向后的最终模型）。

## 三、回归测试

### 单元测试（`testing` + `testify`，置于被测代码旁）

1. `mod_ai_token_auth/model_check_test.go`（新）：`ValidateTargetModel` 直测——nil context 放行、无列表放行、allow 命中/未命中（未命中返回 `CodeModelNotAllowed` 且 `details.Model == targetModel`）、block 命中/未命中、空 targetModel 返回 `CodeInvalidRequest`、`ReqAuthFail` 计数递增、block 优先于 allow。
2. `token_rule_table_test.go` 调整：`ValidateUserTokenByReq` 删除模型校验块后，原直测用例中「白名单外 400」类断言迁移至 `ValidateTargetModel`；Key 识别/启用/过期/配额/子网用例不变。
3. `bfe_server`：`doSingleAIForward` 注入点用例——ModelMapping 重定向后 allow 命中放行（`clusterInvoke` 被调用）、未命中返回 400 且 `aiMeta.TargetModel` 为重定向后模型。

### 集成测试（`bfe/tests/integration`）

- 新场景「模型重定向 × Key 模型白名单」（或扩展 SC16）：issue 复现链路端到端——provider 模型 `glm-5.2` + 集群 `ModelMapping glm-5.2-abc→glm-5.2` + Key `allow_models=["glm-5.2"]` + 请求 `model=glm-5.2-abc` → 200 转发，后端收到 `glm-5.2`（验收 1）；`allow_models=["glm-4"]` → 400，`details.model=glm-5.2`（验收 2）；`block_models` 含 `glm-5.2` → 400，含 `glm-5.2-abc` → 放行（验收 3，block 同样按 target）；无 `ModelMapping` 且无路由目标覆盖时按原始模型校验、行为不退化（验收 4）。
- fallback 复校验（验收 5/6）：主目标 target 未 allow + fallback target 已 allow → 最终 200；均未 allow → 最后 attempt 的 400。可扩展 SC08-ai-model-fallback-recompute（已覆盖 ModelMapping 重算）或 SC02-multi-api-key（已覆盖 ModelMapping）。
- 回归：SC16 全场景（Gemini 路径模型经 `extractClientModel` 继承，#1384 语义不变）、SC04（StripPrefix）、SC08、SC02、SC03（RMB 计费在 `HandleRequestFinish` 按 `aiMeta.TargetModel` 查价——`:1562` 赋值时机不受影响）。

验证命令：`cd bfe && make test`（go test -cover ./... + go vet ./...）。

## 四、修复后正确链路（issue 复现场景）

1. 客户端 `POST /chat/completions`，`model=glm-5.2-abc`，Key `allow_models=["glm-5.2"]`；
2. `HandleFoundProduct`：token 鉴权通过（Key 有效、配额充足；模型校验已不在此阶段）→ `SetTokenAuthContext`；
3. `mod_ai_route` 命中 → `AiRouteResult`（目标集群 + 重定向表）；
4. `doSingleAIForward`：`targetModel = computeTargetModel("glm-5.2-abc", attempt.Model, cluster.AIConf)` → `ModelMapping` 命中 → `"glm-5.2"`；`aiMeta.TargetModel = "glm-5.2"`（:1562）；
5. `ValidateTargetModel(basicReq, "glm-5.2")` → allow 命中 → 放行；
6. 回写 body `model=glm-5.2` → `clusterInvoke` 转发 → 后端收到 `glm-5.2`；日志 `ai_target_model=glm-5.2`、无 `MODEL_NOT_ALLOWED`。

## 五、行为变化清单

| 项 | 改动前 | 改动后 |
| --- | --- | --- |
| 校验对象 | 请求体/路径原始模型（`token_rule_table.go:193,198`） | `computeTargetModel` 输出（`:1559`） |
| 校验时机 | `HandleFoundProduct` 鉴权期（路由前，400 在 `reverseproxy.go:1193-1217` 内短路） | `doSingleAIForward` 内、`clusterInvoke` 前（路由后） |
| 无路由命中 + 模型不允许 | 400 `MODEL_NOT_ALLOWED`（先于 404 短路） | 404 `AI route not found`（鉴权不再短路；issue 原文「透传」表述按代码修正） |
| 鉴权拒绝与 fallback | 鉴权 400 不进入转发，无 fallback 机会 | 本地 400 走集群级 fallback，各 attempt 按各自 target 复校验；全拒则最后的 400 给客户端 |
| key 轮换 | —（鉴权期一次） | 本地 400 命中轮换循环 `default:` 分支即停，不重复校验（`:1766-1768`） |
| 拒绝时 `ai_target_model` | = 请求模型（`http_conn.go:568` 初始值） | = 重定向后目标模型 |
| 错误 `details.model` | 原始模型 | targetModel |
| 成功请求 `ai_auth_reject_reason` | — | fallback 成功时清除（步骤 4），保持为空 |

## 六、验收标准对照（issue 原文）

| # | 验收标准 | 达成方式 |
| --- | --- | --- |
| 1 | 重定向 + `allow_models=[glm-5.2]` + 请求 `glm-5.2-abc` → 200 转发，后端收到 `glm-5.2`，日志 `ai_target_model=glm-5.2` | 步骤 1-3 + 集成场景一 |
| 2 | `allow_models=[glm-4]` → 400，`details.model=glm-5.2` | 步骤 1（`details.Model` 填 targetModel）+ 集成场景一 |
| 3 | `block_models` 按 target 对称校验 | 步骤 1（block/allow 同一 `targetModel` 变量）+ 集成场景一 |
| 4 | 无重定向/无路由目标覆盖 → 按原始模型校验，行为不退化；无路由命中 → 404 | 步骤 1（target==client model 时等价）+ 第五节行为变化；集成场景一 |
| 5 | 多目标/fallback 按实际选中 target 复校验 | 步骤 3（`:1300-1331` 循环内每 attempt 重算重校）+ 集成场景二 |
| 6 | Entity 继承的有效列表同样按 target 生效 | `token.Models`/`BlockModels` 本就是控制面合并后的有效列表，数据面零特殊处理 |
| 7 | 单元/集成测试覆盖 `mod_ai_token_auth` + `bfe_server`（fallback、无路由、block 对称、错误 detail） | 第三节全部 |
| 8 | §9.7 / §5.6 / §10.4 文档表述与新行为一致 | 步骤 5 |

## 七、边界与非目标

- **限流同型缺口（姊妹卡，不在本卡）**：`mod_ai_rate_limit` 的 TPM/RPM/最大并发「适用模型」按 `meta.ClientModel` 匹配（`mod_ai_rate_limit.go:172,190`），与 allow_models 同型缺口；限流计数器按 model 分桶，改动与回归面更大，按 issue 建议单独评估、单独验收。
- **非目标**：`EnableAiGateway=false` 的传统流不经过 `ServeHTTPForAI`，不受影响。
- **配置语义切换提示**：现有「允许模型填客户端模型名」的配置仅在「无路由目标 Model 覆盖 + 无 StripPrefix + 无 ModelMapping」时与改动前等价；一旦启用重定向/裁剪/目标覆盖，校验对象切换为转发后模型——这是本修复意图，但属语义变化，需在 §9.7 显式说明并随版本发布公告。
- **指标语义**：`ReqAuthFail` 计数从鉴权期平移到转发前校验点（步骤 1），总量语义不变（一次拒绝仍只计一次）。

## 八、实施记录（2026-09-23）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_modules/mod_ai_token_auth/model_check.go` | 新增 `ValidateTargetModel(req, targetModel)`：从 `TokenAuthContext` 取有效 allow/block 列表校验**目标模型**；无 context/无列表放行；空 targetModel 保持历史 400 `INVALID_REQUEST` 语义；拒绝时 `SetAiAuthInfo` + `ReqAuthFail` 计数，`details.Model` 填 targetModel（步骤 1） |
| `bfe_modules/mod_ai_token_auth/token_rule_table.go` | 删除 `ValidateUserTokenByReq` 的模型校验块（原 :192-243），鉴权期不再因模型拒绝；连带移除仅被该块使用的 `condition`/`bfe_model_protocol`/`strings` import（步骤 2） |
| `bfe_server/reverseproxy.go` | `doSingleAIForward` 在 `aiMeta.TargetModel = targetModel` 之后、`clusterInvoke` 之前注入 `ValidateTargetModel` 校验（模块获取沿用 `srv.Modules.GetModule` 模式，含 nil 防御）（步骤 3）；`ServeHTTPForAI` fallback 循环成功 break 分支清除残留 `RejectReason`/`RejectQuotaPlans`（步骤 4） |
| 单元测试 | `model_check_test.go` 新增（9 例：无 context、无列表、allow 命中/未命中、block 命中/按 target 不匹配原始名、block 优先、空 target 400、空白 trim、ReqAuthFail 计数、RejectReason、details.Model）；`token_rule_table_test.go` 重写为 `TestValidateUserTokenByReqModelCheckRemoved`（鉴权期模型放行 4 例，helpers 供两测试文件共用） |
| 集成测试 | 新增场景 `scenario-SC18-key-model-allowlist-target-model`（TC-01 重定向后放行/TC-02 拒绝 details.model=target/TC-03 block 对称两子用例/TC-04 无重定向不退化/TC-05 fallback 复校验/TC-06 gemini 路径回归）；设计文档 `scenario-SC18-Key模型白名单按目标模型校验/`（场景说明 + 6 个 TC）并登记进 `测试场景总体说明.md` |

验证：`go test -cover ./...` 全量通过；`go vet` 干净；新文件 gofmt 干净（仓库既存的注释列表缩进 gofmt 差异为存量问题，未触碰）；SC18 全 6 TC 通过（11.1s），SC16（12.7s）、SC08（3.4s）回归通过。

TC-05 实测行为与设计完全一致：主目标本地 400 在 key 轮换循环 `default` 分支停止轮换、因 400 ∈ 默认 `aiFallbackStatusCodes` 触发集群级 fallback；被拒 attempt 后端 Hits()==0（校验在 `clusterInvoke` 之前）；fallback attempt 从 `ClientModel` 重算 targetModel 复校验通过，最终 200 且仅计费一次。

文档同步（另仓 `ai-gateway-web`，步骤 5）：`09-api-key.md` §9.7 重写为两阶段执行模型（限流/配额在鉴权阶段，模型访问控制在转发阶段按目标模型）；`05-ai-business-cluster.md` §5.6 增加与 allow/block 模型组合说明；`10-route.md` §10.4 「指定模型」补充其为目标模型解析第一步、白名单按最终目标模型判定。

实施中对计划的偏离：

1. 步骤 3 注入点增加 `srv.Modules != nil` 防御判断，与 `reverseproxy.go` 既有 `srv.Modules` 使用点（key 亲和性处）的防御风格一致。
2. 单元测试未新增 `doSingleAIForward` 级 harness 用例（该函数需完整 srv/cluster 装配，既有测试文件均为纯函数级）；注入逻辑由 SC18 集成场景端到端覆盖（含 key 轮换不重复触发、fallback 复校验、后端零命中断言）。
