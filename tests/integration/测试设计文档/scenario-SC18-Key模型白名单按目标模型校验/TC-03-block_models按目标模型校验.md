# TC-03 block_models 按目标模型校验

## 用例编号与名称

TC-03 block_models 按目标模型校验（block 原始模型名不影响重定向后请求）

## 所属场景

SC18 Key 模型白名单按目标模型校验

## 版本声明

- `bfe`：issue #1387 修复起（`mod_ai_token_auth.ValidateTargetModel`，block 优先于 allow）

## 测试目的

验证 `block_models` 与 `allow_models` 对称地按重定向后目标模型校验：

- (a) `block_models="glm-5.2"`：目标模型被 block → 400 `MODEL_NOT_ALLOWED`，`details.model=glm-5.2`，请求未到达后端、不扣费；
- (b) `block_models="glm-5.2-abc"`：block 列表只含请求体原始模型名，而校验按目标模型 `glm-5.2` 匹配 → 放行，响应 200，后端收到的 model 为 `glm-5.2`。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（计费断言）。

## 前置条件

1. 同 TC-01 环境：(a) token `ak_user_a` 配置 `block_models="glm-5.2"`；(b) token `ak_user_a` 配置 `block_models="glm-5.2-abc"`。
2. `cluster_redirect` 配置与响应同 TC-01。

## BFE 请求

(a)(b) 均为 POST `http://<bfe>/v1/chat/completions`，Host `redirect.example.org`：

| 字段 | 值 |
|------|----|
| Authorization | Bearer ak_user_a |
| Content-Type | application/json |
| body | `{"model":"glm-5.2-abc","messages":[{"role":"user","content":"hello"}]}` |

## 目标模型计算

同 TC-01：目标模型为 `glm-5.2`。

- (a) `glm-5.2` ∈ block_models → 拒绝；
- (b) `glm-5.2` ∉ block_models("glm-5.2-abc") → 放行。

## 预期结果

请求 (a)：

1. 响应 400，错误体 `error.code=MODEL_NOT_ALLOWED`，`error.details.model="glm-5.2"`；
2. `cluster_redirect` 命中 0 次；
3. `quota:plan_token` 余额不变（仍为 1000000）。

请求 (b)：

1. 响应 200，响应体为后端透传；
2. `cluster_redirect` 命中 1 次，收到的请求体 model 为 `glm-5.2`（重定向照常生效）。

## 关联

- issue：https://github.com/bfenetworks/bfe/issues/1387
- 修复方案：`docs/zh_cn/modifications/2026-09-23-issue-1387-key-model-allowlist-target-model/design-changes.md`
