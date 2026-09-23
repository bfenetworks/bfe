# TC-01 allow_models 按重定向后目标模型放行

## 用例编号与名称

TC-01 allow_models 按重定向后目标模型放行（issue 复现主链路）

## 所属场景

SC18 Key 模型白名单按目标模型校验

## 版本声明

- `bfe`：issue #1387 修复起（`mod_ai_token_auth.ValidateTargetModel` + `doSingleAIForward` 转发前注入校验）

## 测试目的

验证 API Key 配置 `allow_models="glm-5.2"`（重定向后目标模型）时，请求体原始模型 `glm-5.2-abc` 经 `cluster_redirect` 的 `ModelMapping`（`glm-5.2-abc` → `glm-5.2`）重定向后命中白名单：响应 200，后端收到的请求体 model 被回写为 `glm-5.2`，响应体透传并正常计费。修复前鉴权期按请求体原始模型 `glm-5.2-abc` 校验会被 400 误拒（issue #1387 复现路径）。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（计费断言）。

## 前置条件

1. 环境：`mod_ai_route` + `mod_ai_token_auth` + `mod_body_process`，token `ak_user_a` 配置 `allow_models="glm-5.2"` 与 total_token 配额方案 `plan_token`（初始配额 1000000，Redis Key `quota:plan_token`）。
2. `cluster_redirect` 配置 `ModelMapping: {"glm-5.2-abc": "glm-5.2"}`，返回 200 与 `usage.total_tokens=15` 的 openai 非流式响应体。

## BFE 请求

POST `http://<bfe>/v1/chat/completions`，Host `redirect.example.org`：

| 字段 | 值 |
|------|----|
| Authorization | Bearer ak_user_a |
| Content-Type | application/json |
| body | `{"model":"glm-5.2-abc","messages":[{"role":"user","content":"hello"}]}` |

## 目标模型计算

1. 路由目标 Model 覆盖：空，保持 `ClientModel=glm-5.2-abc`；
2. 无 StripPrefix；
3. `cluster_redirect` ModelMapping：`glm-5.2-abc` → `glm-5.2`；
4. 校验目标模型 `glm-5.2` ∈ allow_models → 放行，body model 回写为 `glm-5.2`。

## 预期结果

1. 响应 200，响应体为后端透传（含 `"total_tokens":15`）；
2. `cluster_redirect` 命中 1 次，收到的请求体 model 为 `glm-5.2`（重定向后回写）；
3. 异步扣款完成后（sleep 500ms），`quota:plan_token` 余额 = 1000000 - 15。

## 关联

- issue：https://github.com/bfenetworks/bfe/issues/1387
- 修复方案：`docs/zh_cn/modifications/2026-09-23-issue-1387-key-model-allowlist-target-model/design-changes.md`
