# TC-02 目标模型不在 allow_models 内拒绝

## 用例编号与名称

TC-02 目标模型不在 allow_models 内拒绝（details.model 为目标模型）

## 所属场景

SC18 Key 模型白名单按目标模型校验

## 版本声明

- `bfe`：issue #1387 修复起（`mod_ai_token_auth.ValidateTargetModel` + `doSingleAIForward` 转发前注入校验）

## 测试目的

验证同一路由（重定向后目标模型 `glm-5.2`）下，token `allow_models="glm-4"` 不含目标模型时：响应 400 `MODEL_NOT_ALLOWED`，且 `details.model` 为目标模型 `glm-5.2`（而非请求体原始模型 `glm-5.2-abc`）；请求未到达后端、不产生配额扣减。同时锁定拒绝发生在转发到后端之前（`clusterInvoke` 之前）。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（计费断言）。

## 前置条件

1. 同 TC-01 环境，唯一差异为 token `ak_user_a` 配置 `allow_models="glm-4"`。
2. `cluster_redirect` 配置与响应同 TC-01。

## BFE 请求

POST `http://<bfe>/v1/chat/completions`，Host `redirect.example.org`：

| 字段 | 值 |
|------|----|
| Authorization | Bearer ak_user_a |
| Content-Type | application/json |
| body | `{"model":"glm-5.2-abc","messages":[{"role":"user","content":"hello"}]}` |

## 目标模型计算

同 TC-01：目标模型为 `glm-5.2`；`glm-5.2` ∉ allow_models("glm-4") → 拒绝。

## 预期结果

1. 响应 400，错误体 `error.code=MODEL_NOT_ALLOWED`，`error.details.model="glm-5.2"`（目标模型，而非请求体原始模型 `glm-5.2-abc`）；
2. `cluster_redirect` 命中 0 次（校验点在 `clusterInvoke` 之前，请求未到达后端）；
3. `quota:plan_token` 余额不变（仍为 1000000），拒绝请求不产生扣减。

## 关联

- issue：https://github.com/bfenetworks/bfe/issues/1387
- 修复方案：`docs/zh_cn/modifications/2026-09-23-issue-1387-key-model-allowlist-target-model/design-changes.md`
