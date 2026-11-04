# TC-10 Redis 故障仍计 Redis 错误且不计未命中

## 用例编号与名称

TC-10 Redis 故障仍计 Redis 错误且不计未命中（issue #1472）

## 所属场景

SC10 同 cluster 多 API-Key 会话保持

## 版本声明

- `bfe`：当前源码版本 + issue #1472 修复（`redisGetOrMiss` 只把 `redis.ErrNil` 归一为未命中，其余错误原样上抛）

## 测试目的

验证与 TC-09 互斥的另一半语义：**真实 Redis 故障**仍然被当作故障处理，两类事件在计数与日志上可区分——

1. `ReqAiKeyAffinityRedisErr` **增长**（Redis 操作失败次数）；
2. `ReqAiKeyAffinityMiss` **不变**（故障不得计为未命中）；
3. `bfe.log` 出现 `redis get binding error`（WARN 为真故障保留）；
4. 故障期间请求仍成功（fail-open：退化为加权随机），Redis 恢复后未命中/写绑定路径重新工作。

对应设计条款：`multi_api_key.md` §3.5.5「Redis Get 失败 → 记录错误指标，回退到加权随机」与 §3.5.6 指标表。

## 运行模式

单组件模式：真实 `bfe` 进程 + 嵌入式 Redis（可停可恢复）+ mock 后端。

## 前置条件

1. 已编译含 issue #1472 修复的 `bfe` 可执行文件。
2. `mod_ai_rate_limit/mod_ai_rate_limit.conf` 中 `IsRejectOnRedisError = false`（限流对 Redis 故障 fail-open，请求仍会进入 `aiClusterInvoke`）。
3. 配额计划为 `Unlimited = true`（配额路径不访问 Redis，故障只影响亲和与限流）。
4. `[AIKeyAffinity]` 段已启用；cluster 亲和配置同 TC-09。

## 配置构造

- 使用场景默认配置。
- 故障注入：`common.RedisServer.Fault()` 停止嵌入式 Redis（后续命令连接被拒），`Recover()` 在同一地址重启一个空实例。

## 执行步骤

1. 启动 BFE 与 Redis，发送 1 条请求建立绑定。
2. 采样计数器，确认此时 `MISS = 1`、`REDIS_ERR = 0`。
3. 调用 `Fault()` 制造 Redis 全故障。
4. 再发送 1 条请求，采样计数器并检查 `bfe.log`。
5. 调用 `Recover()` 恢复 Redis。
6. 再发送 1 条请求，采样计数器并读取绑定键。

## 预期结果

**故障期间（步骤 4）**：

- 响应 200（fail-open，转发不受影响）；
- `ReqAiKeyAffinityRedisErr` 相对步骤 2 **增长**（读绑定 1 次；成功转发后的补写还会再计 1 次，因此不要求精确等于 1）；
- `ReqAiKeyAffinityMiss` **与步骤 2 相同**（故障不计为未命中）；
- `bfe.log` 含 `redis get binding error`。

**恢复后（步骤 6）**：

- 响应 200；
- `ReqAiKeyAffinityMiss` 相对故障期 **+1**（恢复后的 Redis 为空，本次是正常未命中）；
- 绑定键重新存在。

## 清理

停止 `bfe` 进程、Redis 与 mock 后端，删除临时目录。

## 备注

- 本用例与 TC-09 构成一对：TC-09 要求"未命中不得计故障"，TC-10 要求"故障不得计未命中"，两者共同锁住 §3.5.5 的互斥定义。
- `Fault()` 后 Redis 数据丢失，`Recover()` 起来的是空实例，因此恢复后必然是一次新绑定，可同时验证"降级后能自动恢复"。
- 计数器读取同样经 `/monitor/proxy_state`；阈值用"增长/不变"而非精确值，避免与"是否发生补写"等实现细节耦合。