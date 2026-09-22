# BFE Issue #1382 修复方案

- Issue: https://github.com/bfenetworks/bfe/issues/1382
- 缺陷：Codex 的 provider-native 路径 `/compatible-mode/v1/responses` 被识别为 Chat 模式而非 Responses 模式，导致 `ai_mode` 记录错误、Responses 价格配置无法命中（价格缺失时静默 0 计费，或按 Chat 价格错计）
- 代码库：`bfe/`（已对照当前代码逐条核实，行号基于 HEAD 2f1b7286）

## 一、根因

### 第 1 层（直接缺陷）：mode 识别不覆盖 provider-native 前缀形态

mode 在请求入口处、上游路径改写**之前**用原始客户端路径判定（`bfe_server/http_conn.go:557`）：

```go
aiMeta.Mode = bfe_basic.DetectModeFromPath(request.HttpRequest.URL.Path)
```

`DetectModeFromPath`（`bfe_basic/request_ai_basic.go:187-192`）→ `StripV1Prefix`（`bfe_basic/openai_endpoint.go:53-58`）**只剥离开头的 `/v1/`**，`/compatible-mode/v1/responses` 原样进入 `lookupOpenAIEndpointMode`（`openai_endpoint.go:73-83`）：精确查表 miss，子路径前缀循环（要求 path 以 `/responses/` 等端点开头）也 miss → 回退 `ModeChat`。

### 第 2 层（后果链）：mode 错 → 价格查询错 → 静默错计/漏计

- 访问日志 `ai_mode` 记为 chat（`bfe_modules/mod_access_pb3/request_log.go:401` 直接取 `aiInfo.Mode`）；
- `calcCostUnits`（`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:549-586`）用 mode 做**精确**匹配查 `LookupModelPrice`（`bfe_config/bfe_cluster_conf/cluster_conf/cluster_conf_load.go:1563-1572`，`priceIndex[model][mode]` 无回退）：
  - 模型只配了 `responses` 价格 → miss → 仅打一条 warn（`mod_ai_token_auth.go:568`）后返回 0 → **静默零计费**；
  - 模型同时配了 chat 与 responses 价格 → 按 chat 价格**错计**。

### 第 3 层（为什么既有修复没覆盖）：改写与 mode 判定共用归一化，语义不同

#1379/#1380 修复了"不带 /v1 的客户端入口"（`/responses`）与路径改写对 provider base 的支持。provider-native 前缀形态（OpenAI SDK base_url `/compatible-mode/v1` 直连入口）在**改写层**被有意当作"已是上游形态"原样透传——`IsOpenAIEndpoint("/compatible-mode/v1/chat/completions") = false` 是显式断言（`openai_endpoint_test.go:57`、`ai_path_rewrite_test.go`），否则改写会把 base 二次拼进路径。但 **mode 检测层**没有对等的前缀归一化，二者共用同一个"只剥开头 /v1"的 `StripV1Prefix`，于是漏了这个形态。

与 #1381 的关系：issue 所述"相互独立"成立，但两者叠加才构成 Codex 完整计费链路——#1381 修 usage 解析（已推 2f1b7286），本 issue 修模式/价格命中。SC03 的 TC-18/19 使用 `/v1/responses`，发现不了本问题。

## 二、修复步骤

### 步骤 1（核心，必改）：`DetectModeFromPath` 增加 provider-native `/v1/` 段识别

文件：`bfe_basic/openai_endpoint.go`（新增 helper）+ `bfe_basic/request_ai_basic.go:187-192`（调用点）

**不改 `StripV1Prefix` 与 `IsOpenAIEndpoint`**：二者被上游路径改写共用（`bfe_server/ai_path_rewrite.go:63-67`），改写对 provider-native 路径的语义是"透传"，必须保持。改为给 mode 判定单独加一个归一化 helper：

```go
// normalizeEndpointLookupPath reduces a client entry path to its OpenAI
// endpoint form for billing-mode lookup: a leading /v1 is stripped
// (standard entry), and a provider-native prefix ending in a /v1 segment
// (the OpenAI SDK base_url form, e.g. /compatible-mode/v1, issue #1382)
// is reduced to the part after that segment. Mode detection only; the
// upstream path rewrite keeps its own StripV1Prefix semantics.
func normalizeEndpointLookupPath(path string) string {
	if rest := StripV1Prefix(path); rest != path {
		return rest
	}
	if i := strings.Index(path, "/v1/"); i >= 0 {
		return StripV1Prefix(path[i+len("/v1"):])
	}
	return path
}
```

`DetectModeFromPath` 改为一行调用替换：`lookupOpenAIEndpointMode(normalizeEndpointLookupPath(path))`。

语义安全分析（只影响"原本落 ModeChat 兜底"的路径，查不到依旧 ModeChat）：

| 路径 | 修复前 | 修复后 | 说明 |
| --- | --- | --- | --- |
| `/compatible-mode/v1/responses` | chat ✗ | **responses** | issue 核心场景 |
| `/compatible-mode/v1/chat/completions` | chat | chat（改由表命中） | 结果不变 |
| `/compatible-mode/v1/embeddings` | chat ✗ | embedding | 附带修正 |
| `/compatible-mode/v1/messages` | chat | chat | `/messages` 不在表中，Anthropic 入口保持默认 |
| `/v1/messages`、`/messages` | chat | chat | 不变 |
| `/v10/xxx`、`/v1beta/models/...` | chat | chat | 不含 `/v1/` 段 |
| `/x/v1/y`（y 非端点） | chat | chat | 二次查表 miss |
| `/a/v1/v1/responses` | chat | responses | 叠加形态，`StripV1Prefix` 二次剥离兜住 |

改写与 mode 判定仍以同一张 `openAIEndpointModes` 表为唯一端点定义，不会分歧（只是各自的"路径归一化"不同，这一点在 helper 注释中写明）。

### 步骤 2（验收 5，必改）：价格缺失显式化（监控可观测）

文件：`bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go:47-49`（计数器声明）、`:566-570`（miss 分支）

现状：`LookupModelPrice` miss 仅打 warn 日志后 0 计费——有日志但无指标，易被淹没，不满足验收 5"明确日志和错误处理"。修改：

1. 模块计数器仿 `ReqAuthFail` 新增 `PriceLookupMiss *metrics.Counter`，在 miss 分支 +1，使价格缺失可监控、可告警；
2. warn 日志保留并补足上下文（已有 cluster/model/mode）；
3. **保持 0 计费、不回改响应**：扣费发生在请求完成之后，无法回改已返回的 200；阻断响应会破坏可用性；0 计费符合 #1352 以来"宁漏收、不错收"的一贯语义。**明确不做**"按 chat 价格兜底计费"——那正是本 issue 要消灭的错误行为。

### 步骤 3（无需改代码，语义确认）：改写层保持原样

`ai_path_rewrite.go` 的 provider-native 透传语义不变（`IsOpenAIEndpoint` 断言不变）；issue 建议 2"保留路径改写前的协议模式信息"结构上已满足（mode 在 `http_conn.go:557` 于改写前判定），无需改动。

## 三、回归测试

1. `bfe_basic/openai_endpoint_test.go`：
   - `TestDetectModeFromPath` 新增臂：`/compatible-mode/v1/responses`→responses、`/compatible-mode/v1/chat/completions`→chat、`/compatible-mode/v1/embeddings`→embedding、`/compatible-mode/v1/models`→chat、`/a/v1/v1/responses`→responses、`/compatible-mode/v1/messages`→chat；
   - 不变臂全量回归：`/v10/xxx`、`/v1beta/models/gemini:generateContent`、`/v1/messages`、`/custom/path`；
   - `TestIsOpenAIEndpoint` / `TestStripV1Prefix` 现有断言**一个不改**（改写语义不回归）。
2. `bfe_server/ai_path_rewrite_test.go`：provider-native 路径透传、标准入口拼 base 用例全量回归（不改代码即应通过）。
3. 集成 SC05（access log）：新增 `/compatible-mode/v1/responses` 请求臂，断言 `ai_mode=responses`（现有用例在 `sc05_access_log_ai_fields_test.go:1120-1139` 已覆盖 `/v1/responses` 臂）。
4. 集成 SC03（RMB 计费）：复用 #1381 的 `responsesAIConf`，新增 `/compatible-mode/v1/responses` 计费臂（SSE，usage 同 TC-18），断言按 responses 价格扣减 900000 定点单位——即"issue #1381 + #1382 叠加后 Codex 真实路径端到端计费"的完整回归。
5. 计数器：如模块单测框架支持，断言价格缺失时 `PriceLookupMiss` 递增。

验证命令：`go test -cover ./bfe_basic/... ./bfe_server/... ./bfe_modules/mod_ai_token_auth/...` + `go vet` + `gofmt`；集成跑 SC03、SC05 全场景。

## 四、修复后正确链路（目标行为）

Codex 请求 `POST /compatible-mode/v1/responses`：

1. `http_conn.go:557`：`normalizeEndpointLookupPath` → `/responses` → `openAIEndpointModes` 命中 → `Mode=responses`，access log `ai_mode=responses`；
2. 上游路径改写：`IsOpenAIEndpoint("/compatible-mode/v1/responses")=false` → 原样透传（上游本来就是该形态）；
3. #1381 修复后 `response.completed` 的 usage 正常解析入 `TokenUsage`；
4. `calcCostUnits`：`LookupModelPrice(table, model, "responses")` 精确命中 → 按 responses 价格扣减；
5. 价格未配置：`PriceLookupMiss` 计数 +1、warn 日志明确 cluster/model/mode，按 0 计费（可监控、不错计）。

## 五、验收标准对照（issue 原文）

| # | 验收标准 | 达成方式 |
| --- | --- | --- |
| 1 | `/v1/responses`、`/responses`、`/compatible-mode/v1/responses` 均识别为 ModeResponses | 前两个 #1379 已满足；第三个步骤 1 |
| 2 | `/v1/chat/completions` 与 `/compatible-mode/v1/chat/completions` 仍识别为 Chat | 步骤 1（结果不变，改由表命中） |
| 3 | 访问日志 `ai_mode=responses` | 步骤 1 + SC05 回归 |
| 4 | 配置 Responses 价格时正确命中 | 步骤 1（mode 修对后 `LookupModelPrice` 精确命中）+ SC03 回归 |
| 5 | 价格缺失有明确日志和错误处理，不静默按 Chat 价格计费 | 步骤 2（计数器 + warn + 0 计费，显式拒绝兜底错计） |
| 6 | 单元测试与接口回归测试 | 第三节全部 |

## 六、边界与非目标

- **非目标**：provider base 不含 `/v1` 的形态（如 `/api/v3/responses`、`/coding/responses`）无法从路径泛化判定，mode 仍走 chat 兜底；如需覆盖应走"端点表配置化/请求体特征"路线，另行设计。
- Anthropic provider 前缀（`/coding/v1/messages`）保持 ModeChat：Anthropic 计费本就挂在 chat 模式价上（SC03 anthropic 用例可证），不属于本 issue 范围。

## 七、实施记录（2026-09-22）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_basic/openai_endpoint.go` | 新增 `normalizeEndpointLookupPath`：先 `StripV1Prefix`，未变则取首个 `/v1/` 段之后子路径并二次 `StripV1Prefix`（兜叠加形态）；`StripV1Prefix`/`IsOpenAIEndpoint`/`lookupOpenAIEndpointMode` 未动（上游改写语义不变） |
| `bfe_basic/request_ai_basic.go` | `DetectModeFromPath` 改经 `normalizeEndpointLookupPath` 查表；注释补充 provider-native 形态（issue #1382） |
| `bfe_modules/mod_ai_token_auth/mod_ai_token_auth.go` | `ModuleAITokenAuthState` 新增 `PriceLookupMiss` 计数器；`calcCostUnits` 价格 miss 分支计数 +1（保持 0 计费与 warn，不做 chat 价格兜底） |
| 单元测试 | `openai_endpoint_test.go`：`TestNormalizeEndpointLookupPath`（9 臂）+ `TestDetectModeFromPath` 新增 7 臂（compatible-mode responses/chat/embeddings/models/messages、叠加形态 `/a/v1/v1/responses`、`/coding/v1/chat/completions`）；`TestIsOpenAIEndpoint`/`TestStripV1Prefix` 断言零改动；`mod_ai_token_auth_test.go` 新增 `TestCalcCostUnits_PriceLookupMissCounter`（miss 计数+0 计费、命中不计数） |
| 集成测试 | SC05 新增 `TestTC17_ModeFieldsProviderNativePath`（ai_mode 断言 + 按真实模式价格扣减 220000，设计文档 `TC-17` 及场景说明已同步）；SC03 新增 `TestTC20_RMBQuotaDeduction_ResponsesAPI_ProviderNativePath`（Codex 真实路径端到端扣减 900000，设计文档 `TC-20` 及场景说明已同步） |

验证：`go test`（bfe_basic/...、bfe_server、mod_ai_token_auth、mod_ai_route、mod_body_process、mod_access_pb3 全部 ok，`bfe_server` 含 ai_path_rewrite 透传语义回归）+ `go vet` + `gofmt` 干净；SC03（TC-01~TC-20）、SC05（TC-01~TC-17）集成全场景通过。

实施中对计划的偏离：无实质偏离。`normalizeEndpointLookupPath` 对 `/v1/` 段后子路径多做一次 `StripV1Prefix`，覆盖计划表中 `/a/v1/v1/responses` 叠加形态（计划已列出，实现顺手兜住）。
