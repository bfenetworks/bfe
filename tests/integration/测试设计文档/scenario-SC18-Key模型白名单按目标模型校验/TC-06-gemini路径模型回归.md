# TC-06 gemini 路径模型回归

## 用例编号与名称

TC-06 gemini 路径模型回归（目标模型继承 ClientModel）

## 所属场景

SC18 Key 模型白名单按目标模型校验

## 版本声明

- `bfe`：issue #1387 修复起（目标模型校验逻辑迁移后，issue #1384 的 gemini 路径模型语义须保持）

## 测试目的

验证 gemini 原生请求（请求体无 `model` 字段，模型在请求路径 `/v1beta/models/{model}:generateContent`）在 issue #1387 修复后语义不回退：`extractClientModel`（body 优先 + gemini 路径兜底）提取路径模型进入 `ClientModel`，无路由 Model 覆盖、无 ModelMapping 时目标模型继承 `ClientModel`；token `allow_models="gemini-2.5-flash"` 命中后放行转发并正常计费。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（计费断言）。

## 前置条件

1. 同 TC-01 环境，唯一差异为 token `ak_user_a` 配置 `allow_models="gemini-2.5-flash"`；Host `gemini.example.org` 路由到 `cluster_gemini`（`ModelProtocols: ["gemini"]`）。
2. `cluster_gemini` 返回 200 与 `usageMetadata.totalTokenCount=15` 的 gemini 非流式响应体。

## BFE 请求

POST `http://<bfe>/v1beta/models/gemini-2.5-flash:generateContent`，Host `gemini.example.org`：

| 字段 | 值 |
|------|----|
| x-goog-api-key | ak_user_a |
| Content-Type | application/json |
| body | `{"contents":[{"parts":[{"text":"hi"}]}]}`（原生 Gemini，无 `model` 字段） |

## 目标模型计算

1. 请求体无 `model` 字段 → `extractClientModel` 从路径提取 `gemini-2.5-flash` → `ClientModel`；
2. 无路由 Model 覆盖、无 StripPrefix、无 ModelMapping；
3. 目标模型 = `gemini-2.5-flash` ∈ allow_models → 放行。

## 预期结果

1. 响应 200，响应体为后端透传（含 `"totalTokenCount":15`）；
2. `cluster_gemini` 命中 1 次；上游收到 `x-goog-api-key: goog-gemini-key`，无 `Authorization`；
3. 异步扣款完成后（sleep 500ms），`quota:plan_token` 余额 = 1000000 - 15。

## 关联

- issue：https://github.com/bfenetworks/bfe/issues/1387（回归保障 issue #1384 语义）
- 修复方案：`docs/zh_cn/modifications/2026-09-23-issue-1387-key-model-allowlist-target-model/design-changes.md`
