# TC-02 本地 429 不轮换 provider key

## 用例编号与名称

TC-02 本地 429 不轮换 provider key（无 penalty key）

## 所属场景

SC19 限流「适用模型」按目标模型匹配

## 版本声明

- `bfe`：issue #1387 姊妹卡修复起（`aiClusterInvoke` key 轮换循环 `ErrCode == bfe_basic.ErrAiRateLimit` 守卫）

## 测试目的

验证本地限流 429 不被误归类为「provider key 被限流」：`cluster_multi` 配置 2 个 provider key、`MaxRetries=1`（轮换可用）、会话亲和 + 429 惩罚开启。第 2 次请求被本地策略 429 后，BFE 不得轮换 provider key——否则换 key 后请求被放行（限流被绕过），且会生成 affinity penalty key。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis（限流计数 + penalty key 断言）。

## 前置条件

1. `cluster_multi`：2 个 provider key（`multi-key-1` / `multi-key-2`），`ModelMapping: {"glm-5.2-abc": "glm-5.2"}`，`KeyPolicy.MaxRetries=1`、`SessionAffinity=true`、`SessionAffinityPenaltyEnable=true`。
2. 限流策略 `rlp-sc19`（同 TC-01）绑定 `ak_rl`。
3. 后端返回 200 与 `usage.total_tokens=15`。

## BFE 请求

POST `http://<bfe>/v1/chat/completions`，Host `multi.example.org`（共 2 次）：

| 字段 | 值 |
|------|----|
| Authorization | Bearer ak_rl |
| body | `{"model":"glm-5.2-abc",...}` |

## 预期结果

1. 第 1 次响应 200；
2. 第 2 次响应 429，`error.code=RPM_LIMIT_EXCEEDED`；
3. `cluster_multi` 后端 Hits 仍为 1（未轮换、未转发）；
4. 后端只观察到 1 个不同的 Authorization header（未换 key）；
5. Redis 中不存在 `:penalty:` 后缀的 affinity penalty key（无 `bfe:ai:key_affinity:penalty:cluster_multi:*`）。

## 关联

- 修复方案 §2.3 步骤 3（key 轮换守卫）
