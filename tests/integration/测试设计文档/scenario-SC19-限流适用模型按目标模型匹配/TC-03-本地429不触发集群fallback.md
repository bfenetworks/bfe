# TC-03 本地 429 不触发集群 fallback

## 用例编号与名称

TC-03 本地 429 不触发集群 fallback

## 所属场景

SC19 限流「适用模型」按目标模型匹配

## 版本声明

- `bfe`：issue #1387 姊妹卡修复起（fallback 循环 `shouldTriggerFallback` 之前 `ErrCode == bfe_basic.ErrAiRateLimit` 守卫 break）

## 测试目的

验证本地限流 429 不触发集群级 fallback：429 在默认 `aiFallbackStatusCodes` 内，主目标策略 429 时若触发 fallback，fallback attempt 会因幂等守卫跳过复校、直接把请求转发到 fallback 集群（限流被绕过）。守卫要求直接 break，客户端仍收到 429，fallback 集群后端 0 命中。

## 运行模式

单组件模式：真实 `bfe` 进程 + 进程内 miniredis。

## 前置条件

1. `fallback.example.org`：主目标 `cluster_main` + fallback `cluster_backup`（均单 key、无 ModelMapping，后端均 200）。
2. 限流策略 `rlp-sc19`（RPM=1，`models=[glm-5.2]`）绑定 `ak_rl`。

## BFE 请求

POST `http://<bfe>/v1/chat/completions`，Host `fallback.example.org`，body `{"model":"glm-5.2",...}`（共 2 次）。

## 预期结果

1. 第 1 次响应 200，`cluster_main` 命中 1 次；
2. 第 2 次响应 429（`RPM_LIMIT_EXCEEDED`）；
3. `cluster_main` Hits 仍为 1，`cluster_backup` Hits 为 0（未触发集群 fallback）。

## 关联

- 修复方案 §2.3 步骤 3（fallback 守卫）
