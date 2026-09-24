# TC-09 token allow_models 白名单对 gemini 路径模型的协议感知校验

## 用例编号与名称

TC-09 token allow_models 白名单对 gemini 路径模型的协议感知校验

## 所属场景

SC16 Gemini 协议支持

## 版本声明

- `bfe`：issue #1384 修复起（`bfe_model_protocol.ExtractModelFromPath` + token-auth 白名单路径兜底）

## 测试目的

验证 API Key 配置 `allow_models` 白名单时，对 Gemini 原生请求（model 在请求路径、请求体无 `model` 字段）的白名单校验按路径模型执行：白名单内的路径模型放行转发并正常计费；白名单外的路径模型拒绝（`MODEL_NOT_ALLOWED`）且不到达后端、不产生扣减。修复前该类请求被 400 `Model not found in request body` 误拒（issue #1384）。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（计费断言）。

## 前置条件

1. 同 TC-01 环境（`mod_ai_route` + `mod_ai_token_auth` + `mod_body_process`）。
2. token `ak_user_a` 配置 `allow_models="gemini-2.5-flash"`（唯一与本场景其他用例的差异，由 `newTestEnvWithAllowModels` 构建）。
3. `cluster_gemini_only` 返回 200 与 `usageMetadata.totalTokenCount=15` 的 gemini 非流式响应体。

## BFE 请求

请求 1（白名单内）：

POST `http://<bfe>/v1beta/models/gemini-2.5-flash:generateContent`，Host `gemini.example.org`：

| 字段 | 值 |
|------|----|
| x-goog-api-key | ak_user_a |
| Content-Type | application/json |
| body | `{"contents":[{"parts":[{"text":"hi"}]}]}`（原生 Gemini，无 `model` 字段） |

请求 2（白名单外）：同上，路径为 `/v1beta/models/gemini-1.5-pro:generateContent`。

## 预期结果

请求 1：

1. 响应 200，响应体含 `"totalTokenCount":15`（透传）；
2. `cluster_gemini_only` 命中 1 次；上游收到 `x-goog-api-key: goog-gemini-key`，未收到 `Authorization`；
3. 异步扣款完成后（sleep 500ms），`quota:plan_token` 余额 = 1000000 - 15。

请求 2：

1. 响应 400，错误码 `MODEL_NOT_ALLOWED`（`details.model=gemini-1.5-pro`）；
2. `cluster_gemini_only` 命中次数不变（仍为 1），请求未到达后端；
3. `quota:plan_token` 余额不变（仍为 1000000 - 15），拒绝请求不产生扣减。

## 关联

- issue：https://github.com/bfenetworks/bfe/issues/1384
- 修复方案：`docs/zh_cn/modifications/2026-09-23-issue-1384-gemini-token-auth-model-whitelist/design-changes.md`
