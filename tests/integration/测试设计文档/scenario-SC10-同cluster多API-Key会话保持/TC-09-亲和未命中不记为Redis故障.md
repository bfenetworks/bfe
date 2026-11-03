# TC-09 亲和未命中不记为 Redis 故障

## 用例编号与名称

TC-09 亲和未命中不记为 Redis 故障（issue #1472）

## 所属场景

SC10 同 cluster 多 API-Key 会话保持

## 版本声明

- `bfe`：当前源码版本 + issue #1472 修复（`redisGetBinding` 用 `redis_client.IsKeyNotFound` 归一未命中；对应实现 `redisGetOrMiss`）

## 测试目的

验证会话首触语义：某个 `ClientKeyId` 的**首次请求**（Redis 中尚无绑定）在契约上是"未命中"，不是"Redis 操作失败"——

1. `ReqAiKeyAffinityMiss` **+1**（指标表定义：未命中 Redis 绑定次数）；
2. `ReqAiKeyAffinityRedisErr` **不增**（该指标只表示 Redis 操作失败）；
3. `bfe.log` **不出现** `redis get binding error[redigo: nil returned]`；
4. 绑定在请求过程中**被写入** Redis（不依赖转发成功后补写），第二次请求即命中。

对应的设计条款：`docs/zh_cn/sys_design/multi_api_key.md` §3.5.3 第 4 条（未命中须按权重选择并写入新绑定）、§3.5.5（降级仅适用于 Redis Get 失败）、§3.5.6 指标表。

## 运行模式

单组件模式：真实 `bfe` 进程 + 嵌入式 Redis + mock 后端。

## 前置条件

1. 已编译含 issue #1472 修复的 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动且为空（无任何 `bfe:ai:key_affinity:*` 键）。
3. `[AIKeyAffinity]` 段已配置并启用（`Disabled = false`）。
4. cluster 配置：`Keys` = key-a/key-b/key-c（50/30/20），`SessionAffinity=true`、`SessionAffinityTTL=300`、`SessionAffinityRedisPrefix="bfe:ai:key_affinity"`、`SessionAffinityPenaltyEnable=true`。
5. `token_rule.data` 中 `ak_session_affinity` 的 `key_id = session_affinity_key_id`。

## 配置构造

- 使用场景默认配置（见 `场景说明.md` §4.1）。
- mock 后端默认返回 200 `{"ok":true}`。

## BFE 请求

| 字段 | 请求 1 | 请求 2 |
|------|--------|--------|
| Host | `session-affinity.example.org` | 同左 |
| Path | `/v1/chat/completions` | 同左 |
| Authorization | `Bearer ak_session_affinity` | 同左 |
| Body | `{"model":"gpt-4"}` | 同左 |

## 执行步骤

1. 启动 BFE（亲和开启 + 惩罚开启），确认嵌入式 Redis 为空。
2. 采样 `proxy_state` 的 `ReqAiKeyAffinityHit` / `ReqAiKeyAffinityMiss` / `ReqAiKeyAffinityRedisErr` 作为基线。
3. 发送请求 1。
4. 再次采样三个计数器，并读取 Redis 中的绑定键 `bfe:ai:key_affinity:cluster_session_affinity:session_affinity_key_id`。
5. 检查 `bfe.log`。
6. 发送请求 2，再次采样计数器。

## 预期结果

**请求 1**：

- 响应 200；
- `ReqAiKeyAffinityMiss` 增量 **1**，`ReqAiKeyAffinityRedisErr` 增量 **0**，`ReqAiKeyAffinityHit` 增量 **0**；
- 绑定键存在，值为 `key-a`/`key-b`/`key-c` 之一；
- `bfe.log` 不含 `redis get binding error`。

**请求 2**：

- 响应 200；
- `ReqAiKeyAffinityHit` 增量 **1**，`ReqAiKeyAffinityMiss` 与 `ReqAiKeyAffinityRedisErr` 均不增；
- mock 后端收到的两次 `Authorization` 头完全一致（均等于绑定键对应的 Provider Key 密文）。

## 清理

停止 `bfe` 进程、Redis 与 mock 后端，删除临时目录。

## 备注

- 修复前本用例的实际表现（即 issue 的复现）：`MISS` 增量 0、`REDIS_ERR` 增量 1、日志出现 `redis get binding error[redigo: nil returned]`。
- 计数器通过 BFE web monitor `/monitor/proxy_state?format=prometheus` 读取（`common.ModuleMetric(e.bfeMonitorPort, "proxy_state", field)`），字段名按 SNAKE_CASE 渲染。
- 保持"失败"语义的对照用例见 TC-10（真 Redis 故障）。