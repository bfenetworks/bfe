# BFE Issue #1384 修复方案

- Issue: https://github.com/bfenetworks/bfe/issues/1384
- 缺陷：API Key 配置了 `allow_models` / `block_models` 时，`mod_ai_token_auth` 的 model 白名单/黑名单校验仅从请求体 JSON 提取 `model`；Gemini 原生协议的 model 在请求路径（`/v1beta/models/{model}:generateContent`），请求体无 `model` 字段，导致合法 Gemini 请求被 400 `Model not found in request body: <nil>` 拒绝、无法转发到受控后端
- 代码库：`bfe/`（已对照当前代码逐条核实，行号基于 HEAD `4041c24f`）
- 关联设计：`docs/zh_cn/modifications/2026-09-09-gemini-protocol-support/`（Gemini 协议支持，本缺陷为该变更遗漏的数据面校验点）

## 一、根因

`ValidateUserTokenByReq`（`bfe_modules/mod_ai_token_auth/token_rule_table.go:191-234`）的 model 校验硬编码为 body-only：

```go
if len(token.Models) > 0 || len(token.BlockModels) > 0 {
    model, err := condition.ReqBodyJsonFetch(req, "model", nil)  // :192 仅从请求体取
    if err != nil || model == "" {
        // :194-200 直接 400 INVALID_REQUEST "Model not found in request body"
    }
    ...
}
```

三层对照说明这是 Gemini 支持（2026-09-09）落地时的遗漏，而非设计取舍：

1. **协议识别已协议感知**：`bfe_model_protocol/detect.go:66-73` 已按路径识别 Gemini（`:generateContent` / `:streamGenerateContent` / `/v1beta/models/` 前缀）；
2. **usage 提取已协议感知**：`bfe_model_protocol/gemini/usage.go` 按 Gemini 原生 `usageMetadata`（camelCase）提取计费字段；
3. **唯独 token-auth 的 model 白名单校验没有协议感知入口**：`bfe_model_protocol/gemini/` 只实现了 `Key/ExtraHeaders/ErrorNormalizer/InjectAuth/ExtractUsageFields/IsStreamTerminal/IsFinalUsageEvent`，无任何 model 提取方法；`token_rule_table.go:192` 也无路径兜底。

Gemini 原生请求体只有 `contents`/`generationConfig`，`ReqBodyJsonFetch(req, "model", nil)` 返回 `""` → 命中 `:193` 的 400 分支。该行为直接违反 Gemini 协议支持变更摘要的目标 G2（"gemini 客户端请求被 BFE 正确识别并按 gemini 协议转发"）；已批准已知 gap 仅限**错误体风格**为 OpenAI 形态，不包含对合法 Gemini 请求的 400 拒绝。

影响面（与 issue 原文一致）：仅配置 `allow_models`/`block_models` 的 Key 受影响；未配置白名单/黑名单的 Key 不进入该校验分支。语义上是"误拒"（带白名单的 Key 的全部合法 Gemini 请求被 400），白名单安全语义在 Gemini 协议下未生效。

## 二、修复步骤

### 步骤 1（必改）：gemini 适配器新增路径 model 提取

新文件 `bfe/bfe_model_protocol/gemini/model.go`：

```go
// ExtractModelFromPath 从 Gemini 原生请求路径中提取 model 名：
//   /v1beta/models/{model}[:action]
// action 包括 :generateContent / :streamGenerateContent / :countTokens 等。
// 路径不匹配（非 /v1beta/models/ 前缀、model 段为空）返回 ""。
func ExtractModelFromPath(path string) string
```

实现要点：

- 锚定前缀 `/v1beta/models/`——与 `detect.go:69-72` 的路径识别规则同源，识别面不扩大；
- 取前缀之后的剩余部分，按**第一个** `:` 截断（动作后缀），再 trim 空白与尾斜杠；
- 只依赖标准库字符串处理，不 import `bfe_basic`/`bfe_config`/`bfe_modules`（遵守 `docs/zh_cn/sys_design/model_protocol_adapter.md` §3.1 的依赖约束：`bfe_model_protocol` 只允许 import `bfe_http` 与 gjson，否则与 `bfe_basic → bfe_model_protocol` 形成环）；
- 纯函数、无状态，与适配器现有风格一致。

### 步骤 2（必改）：根包导出统一入口

新文件 `bfe/bfe_model_protocol/model.go`：

```go
// ExtractModelFromPath 返回"model 承载在请求路径上"的协议（当前仅 Gemini）
// 从请求路径解析出的 model 名；其他路径返回 ""。nil 请求/nil URL 安全返回 ""。
func ExtractModelFromPath(req *bfe_http.Request) string
```

内部委托 `gemini.ExtractModelFromPath(req.URL.Path)`。加这一层是为了让 `bfe_modules/` 调用方只依赖根包 `bfe_model_protocol`（与 registry/usage/errors 的 alias 模式一致）；未来若出现第二个路径承载 model 的协议，在同一入口扩展。

### 步骤 3（必改）：token-auth 白名单/黑名单校验改为 body-first + 协议路径兜底

`bfe_modules/mod_ai_token_auth/token_rule_table.go:191-200`：

```go
model, err := condition.ReqBodyJsonFetch(req, "model", nil)
if err != nil || model == "" {
    // issue #1384: gemini 原生协议的 model 在请求路径，请求体无 model 字段
    if m := bfe_model_protocol.ExtractModelFromPath(req.HttpRequest); m != "" {
        model, err = m, nil
    }
}
if err != nil || model == "" {
    // 原 :194-200 的 400 分支原样保留（code/type/message/details 均不变）
}
```

语义点：

1. **只做追加兜底、不做替换**：body 提取优先，`jsoncache.` 缓存键、`TrimSpace`、错误消息对 openai/anthropic 零影响——现有协议行为逐字节不变；
2. **白名单与黑名单一次修复**：`models`（:216-233）与 `block_models`（:202-214）共用同一 `model` 变量；
3. **无需以 AuthStyle 为前置条件**：`ExtractModelFromPath` 只对 `/v1beta/models/` 前缀生效，天然限定 gemini 路径；`Authorization: Bearer` 打到 gemini 路径的畸形请求也会按路径模型校验（更严格，不是绕过）；
4. **400 兜底消息不改**："Model not found in request body: %v" 保留。网关自身错误体恒为 OpenAI 风格是 Gemini 变更摘要 §4 已批准的已知 gap（gemini 客户端收到 OpenAI 形态错误体），含本修复保留的这条，不在本次范围；
5. 此时代理尚未改写上游路径（路径改写发生在反向代理转发阶段），`req.HttpRequest.URL.Path` 仍是客户端原始路径，提取结果正确。

### 步骤 4（推荐同修，可独立提交）：`serveRequest` 的 ClientModel 提取同步协议感知

`bfe_server/http_conn.go:551-555` 同样改为 body-first + `ExtractModelFromPath` 兜底填充 `aiMeta.ClientModel` / `aiMeta.TargetModel`。

收益：access log 的 ClientModel 对 Gemini 有值；RMB 按模型计价链路（`calcCostUnits` → `LookupModelPrice(targetModel, mode)`，`mod_ai_token_auth.go:553-577`）对 Gemini 命中模型价格表。

**必须显式知会的语义变化**：此前 Gemini 请求 ClientModel/TargetModel 为空，RMB 计费在 `:558` 直接返回 0（或价格表 miss 记 0）；修复后按路径模型查价计费。行为更正确，但属计费语义变化，需 SC16 + SC03 全量回归；如求保守可拆为独立变更随下一迭代上线。token 配额（total_token）计费路径不读 TargetModel（SC16 TC-01 已验证），不受影响。

## 三、回归测试

新增用例（均放在被测代码旁的 `_test.go`，沿用 `testing` + `testify`）：

1. `bfe_model_protocol/gemini/model_test.go`（新）：表驱动 `ExtractModelFromPath`——
   - `/v1beta/models/gemini-2.5-flash:generateContent` / `:streamGenerateContent` / `:countTokens` / 裸路径 / 尾斜杠 → `gemini-2.5-flash`；
   - `/v1beta/models/`、`:generateContent` 紧跟前缀、空串、非 gemini 路径（`/v1/chat/completions`、`/v1/messages`）→ `""`。
2. `bfe_model_protocol/model_test.go`（新）：nil req、nil URL、非 gemini 路径 → `""`。
3. `bfe_modules/mod_ai_token_auth/token_rule_table_test.go`（新，首个 `ValidateUserTokenByReq` 直测）：
   - Gemini 路径 + 原生 body（无 model）+ `Models` 含路径模型 → 放行返回 token；
   - Gemini 路径 + `Models` 不含路径模型 → `CodeModelNotAllowed`，`details.Model` = 路径模型；
   - Gemini 路径 + `BlockModels` 命中路径模型 → `CodeModelNotAllowed`；
   - openai body（`model` 字段）+ 白名单 → 与现状一致（body 优先于路径）；
   - 非 gemini 路径 + 无 model body → 仍 400 `INVALID_REQUEST`，消息文本不变；
   - 构造要点：token `UnlimitedQuota=true` 绕开 redis；`ruleTable.productTokens` 直接填充（`GetToken` 只读该表）。
4. 步骤 4 若同修：`bfe_server` 增加 gemini 请求 ClientModel 断言用例（复用现有 serveRequest 测试构造）。
5. 集成测试：`bfe/tests/integration` SC16 新增 TC-09——token 配置 `allow_models` + Gemini 路径请求：白名单内模型 200 转发且计费正确、白名单外模型 400 `MODEL_NOT_ALLOWED`；`integration-test` 仓库 SC1401-TC008（issue 发现用例）复跑通过。SC16 TC-01~TC-08、SC03 全场景回归。

验证命令：`cd bfe && make test`（go test -cover ./... + go vet ./...）。

## 四、修复后正确路径（目标行为）

1. 客户端 `POST /v1beta/models/gemini-2.5-flash:generateContent`，header `x-goog-api-key: <key>`，body 为原生 Gemini `{contents, generationConfig}`；
2. `GetApiKey`（`bfe_basic/request_ai_basic.go:158`）识别 key 并置 `AuthStyle=gemini`；
3. token-auth：body 无 `model` → `ExtractModelFromPath` 提取 `gemini-2.5-flash` → `allow_models`/`block_models` 校验通过；
4. `mod_ai_route` 按 path 条件命中 route rule → cluster `ModelProtocols:["gemini"]` 校验（SC16 TC-03/TC-04 语义不变）→ 转发受控后端；
5. `mod_body_process` 按 gemini `usageMetadata` 提取 usage（既有路径）→ 配额扣减；（步骤 4 后）RMB 按路径模型命中价格表计费。

## 五、验收标准对照（issue 原文）

| # | 验收标准 | 达成方式 |
| --- | --- | --- |
| 1 | 带 `models` 白名单的 Key 不再 400 拒绝合法 Gemini 请求，转发到受控后端返回 200 | 步骤 1-3 |
| 2 | 白名单校验协议感知（与 usage 提取同样协议感知） | 步骤 1-3（`bfe_model_protocol` 统一入口） |
| 3 | `models` 与 `block_models` 均生效 | 步骤 3 共用同一 `model` 变量 |
| 4 | openai/anthropic 现有校验零回归 | body 优先 + 第三节回归用例 |
| 5 | 与 `DetectProtocol` 路径识别规则一致 | 同源 `/v1beta/models/` 前缀，识别面不扩大 |

## 六、同类风险排查（本次不修，留作记录）

- **condition 的 model 原语 body-only**：`bfe_basic/condition` 中按 body `model` 取值的 primitive（如 model 匹配类条件）对 Gemini 同样取不到值；Gemini 路由依赖 path 条件（issue 复现步骤即用 path 匹配 route rule），按 model 条件的路由/限流规则在 Gemini 下不生效属配置侧规避场景，如需支持另起评审；
- **Vertex AI 路径形态**：`/{location}/publishers/google/models/{model}:generateContent` 不在 `DetectProtocol` 支持面内（与 issue #1382 的 provider-native 路径支持范围一致），`ExtractModelFromPath` 同样不支持；如需支持随协议识别变更一起扩展；
- **错误体风格**：网关自身错误恒为 OpenAI 形态（含本修复保留的 400 兜底），系 Gemini 变更摘要 §4 已批准已知 gap，需按 AuthStyle 输出 gemini 原生错误体时另起评审。

文档同步：`docs/zh_cn/sys_design/model_protocol_adapter.md` §3.1 包结构图增加 `gemini/model.go` 与根包 `model.go`，接口小节补充 `ExtractModelFromPath` 职责说明。

## 七、实施记录（2026-09-23）

已按上述方案实施，变更文件：

| 文件 | 改动 |
| --- | --- |
| `bfe_model_protocol/gemini/model.go` | 新增 `ExtractModelFromPath(path)`：`/v1beta/models/` 前缀锚定 + 第一个 `:` 截断动作后缀 + 空白/尾斜杠清理；不匹配返回 ""（步骤 1） |
| `bfe_model_protocol/model.go` | 新增根包入口 `ExtractModelFromPath(req)`：nil 请求/nil URL 安全，委托 gemini 子包（步骤 2） |
| `bfe_modules/mod_ai_token_auth/token_rule_table.go` | `ValidateUserTokenByReq` model 校验在 body 提取失败/为空时兜底 `bfe_model_protocol.ExtractModelFromPath`；models/block_models 共用变量一次覆盖；400 兜底消息与 details 结构不变（步骤 3） |
| `bfe_server/http_conn.go` | 抽出 `extractClientModel`（body-first + 路径兜底），`serveRequest` 的 ClientModel/TargetModel 填充改经该 helper（步骤 4）；对既有请求行为等价（body 有值或为空时赋值结果与原先 `err == nil \|\| len(model) > 0` 分支一致） |
| 单元测试 | `gemini/model_test.go`（表驱动 12 例）；`bfe_model_protocol/model_test.go`（nil/入口 5 例）；`mod_ai_token_auth/token_rule_table_test.go` 新增（`ValidateUserTokenByReq` 首个直测：gemini 路径放行/白名单外 400/details.Model/block_models/streamGenerateContent/body 优先/openai 行为不变/空前缀 400 共 7 例）；`bfe_server/http_conn_ai_test.go` 新增 `TestExtractClientModel` 4 例 |
| 集成测试 | SC16 新增 `TestTC09_TokenAllowModelsGeminiPathModel`（token `allow_models="gemini-2.5-flash"`：原生 body 无 model 字段时白名单内路径模型 200 转发 + 按 totalTokenCount=15 扣减；白名单外路径模型 400 `MODEL_NOT_ALLOWED` 且零命中零扣减）；env 构造抽 `newTestEnvWithAllowModels`；设计文档 `TC-09-token-allow_models白名单对gemini路径模型校验.md` 与场景说明已同步 |

验证：`go vet ./...` 干净；`go test -cover ./...` 全量通过（SC01 首次并发跑出现 Windows 二进制文件锁 flake `used by another process`，单独重跑通过，与本变更无关）；SC16 全场景（TC-01~TC-09）通过；gofmt 干净。

实施中对计划的偏离：

1. 步骤 4 原方案为内联修改 `http_conn.go` 提取块；实施时抽为 `extractClientModel` helper 以便直接单测，行为与原条件等价。
2. SC16 TC-09 原方案为"白名单内 200 + 白名单外 400"两步合并为一个测试函数顺序执行（共用同一 env，第二次请求复用第一次的扣减后余额断言零扣减），语义覆盖不变。
