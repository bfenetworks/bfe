# TC-05 fallback 复校验目标模型

## 用例编号与名称

TC-05 fallback 复校验目标模型（主目标校验失败未达后端，fallback 重算后放行）

## 所属场景

SC18 Key 模型白名单按目标模型校验

## 版本声明

- `bfe`：issue #1387 修复起（校验点注入 `doSingleAIForward`；`ServeHTTPForAI` fallback 成功分支清除残留 RejectReason）

## 测试目的

验证集群级 fallback 时模型校验按每个 attempt 的目标模型复算：主目标 `cluster_plain`（无重定向，目标模型 `glm-5.2-abc`）校验失败产生本地 400——该校验在 `clusterInvoke` 之前，主目标后端未收到请求；400 在默认 `aiFallbackStatusCodes` 内触发集群级 fallback，fallback attempt（`cluster_redirect`，重定向后目标模型 `glm-5.2`）重算目标模型并复校验通过后转发成功。

同时锁定两个语义细节：

1. 被拒 attempt 的后端命中数为 0（校验失败发生在转发到后端之前）；
2. 成功 fallback 后残留的主目标 `RejectReason` 被清除（访问日志不携带过期拒绝原因；访问日志断言本场景不做，由实现保证）。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（计费断言）。

## 前置条件

1. 同 TC-01 环境，唯一差异为路由：Host `fallback.example.org` 命中规则 `user_a-fallback`——主目标 `cluster_plain`（无 ModelMapping）+ fallback `cluster_redirect`（ModelMapping `glm-5.2-abc` → `glm-5.2`）；token `ak_user_a` 配置 `allow_models="glm-5.2"`。
2. 两个 cluster 均返回 200 与 `usage.total_tokens=15` 的 openai 非流式响应体。

## BFE 请求

POST `http://<bfe>/v1/chat/completions`，Host `fallback.example.org`：

| 字段 | 值 |
|------|----|
| Authorization | Bearer ak_user_a |
| Content-Type | application/json |
| body | `{"model":"glm-5.2-abc","messages":[{"role":"user","content":"hello"}]}` |

## 目标模型计算

主目标 attempt（cluster_plain）：

1. 无路由 Model 覆盖、无 StripPrefix、无 ModelMapping；
2. 目标模型 = `glm-5.2-abc` ∉ allow_models("glm-5.2") → 本地 400 `MODEL_NOT_ALLOWED`（clusterInvoke 之前，后端未收到请求）；
3. 400 ∈ 默认 `aiFallbackStatusCodes` → 触发集群级 fallback。

fallback attempt（cluster_redirect）：

1. 从 `ClientModel=glm-5.2-abc` 重算（不继承主目标 TargetModel）；
2. ModelMapping：`glm-5.2-abc` → `glm-5.2`；
3. 目标模型 `glm-5.2` ∈ allow_models → 放行，转发成功。

## 预期结果

1. 最终响应 200，响应体为 `cluster_redirect` 后端透传（含 `"total_tokens":15`）；
2. `cluster_plain` 命中 0 次（主目标校验失败发生在 `clusterInvoke` 之前）；
3. `cluster_redirect` 命中 1 次，收到的请求体 model 为 `glm-5.2`；
4. 异步扣款完成后（sleep 500ms），`quota:plan_token` 余额 = 1000000 - 15（仅按最终成功响应计费一次）。

> 说明：若实现将校验放在 `clusterInvoke` 之后，主目标后端将收到请求（Hits=1），测试会失败；若 fallback 未触发（400 未进入 `aiFallbackStatusCodes`），最终响应将为 400，测试会失败。

## 关联

- issue：https://github.com/bfenetworks/bfe/issues/1387
- 修复方案：`docs/zh_cn/modifications/2026-09-23-issue-1387-key-model-allowlist-target-model/design-changes.md`
