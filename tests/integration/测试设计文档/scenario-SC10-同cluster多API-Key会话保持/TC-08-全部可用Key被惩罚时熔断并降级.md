# TC-08 全部可用 Key 被惩罚时熔断并降级到备用集群

## 用例编号与名称

TC-08 全部可用 Key 被惩罚时熔断并降级到备用集群（issue #1439 方案乙）

## 所属场景

SC10 同 cluster 多 API-Key 会话保持

## 版本声明

- `bfe`：v1.8.8 分支 + issue #1439 修复（`filterPenaltyKeys` 排除 Weight=0 且删除"回退全量"兜底）+ issue #1440 修复（耗尽返回合成 429 触发降级）

## 测试目的

验证**熔断语义**（2026-10-21 裁定，方案乙）：当集群全部可用 Key 都处在惩罚窗口内时——

- 惩罚过滤器**严格剔除**被惩罚 Key，不做"回退到被惩罚 Key"的兜底；
- 惩罚窗口内的新请求**与目标集群零交互**，`aiClusterInvoke` 选 Key 失败 → 合成 429 → 外层 `shouldTriggerFallback` 命中白名单 → **降级到备用集群**并返回 2xx；
- `bfe.log` 出现熔断日志（`aiKeyAffinity: all usable keys penalized ... circuit break`）与降级日志（`fallback triggered ... status[429]`）；
- 不出现 0 上游交互的 500。

与 TC-03 的区别：TC-03 验证"单个 Key 被 429 后轮换到其他可用 Key 并成功"；本用例验证"所有可用 Key 都被惩罚时的熔断降级"。

## 运行模式

单组件模式：真实 `bfe` 进程 + 真实 Redis（`common.RedisServer`）+ mock 后端 × 2。

## 前置条件

1. 已编译含 issue #1439/#1440 修复的 `bfe` 可执行文件。
2. Redis 与两个 mock 后端（目标集群 + 备用集群）已启动。
3. `mod_ai_rate_limit` 已加载并提供 Redis 句柄（SC10 既有能力）。
4. 场景 testdata 的 `ai_route.data` 默认规则已配置 `fallbacks=[cluster_session_affinity_fallback]`，且 testEnv 始终创建备用集群后端（本 TC 落地时一并修改，不影响 TC-01~TC-06——它们在目标集群上均成功，fallback 不被触发）。

## 配置构造

- `cluster_session_affinity.AIConf.Keys`：`[{key-a,100},{key-b,100}]`（2 个可用 Key）；
- `KeyPolicy`：`SessionAffinity=true`、`SessionAffinityPenaltyEnable=true`、`SessionAffinityTTL=300`、`MaxRetries=3`（SC10 默认）；
- 目标集群后端 `ResponseFunc`：对任何 Authorization 头一律返回 429 `{"error":"rate limited"}`；
- 备用集群后端：返回 200 `{"ok":true}`。

## BFE 请求

| 字段 | 请求 1 | 请求 2（惩罚窗口内） |
|------|--------|----------------------|
| Host | `session-affinity.example.org` | 同左 |
| Path | `/v1/chat/completions` | 同左 |
| Authorization | `Bearer ak_session_affinity` | 同左 |
| Body | `{"model":"gpt-4"}` | 同左 |

## 执行步骤

1. 以上述配置启动 BFE（亲和开启 + 惩罚开启 + fallback 路由）。
2. **请求 1**：预期在目标集群上做 Key 级轮换（2 个 Key 各被尝试，全部 429），Key 级重试耗尽后返回最后一次 429；两个 Key 均被写入 60s 惩罚键。
3. 记录目标集群后端命中数 `hits1`。
4. **请求 2**（立即发送，远在 60s 窗口内）：观察响应与后端命中数。
5. 检查 `bfe.log`。

## 预期结果

**请求 1**：

- 目标集群后端命中数 ≥ 2（2 个 Key 都被轮换到，全部 429；`MaxRetries=3` 时实际可能为 4 次，`usedSet` 全用过后会重置再试）；
- `aiClusterInvoke` 返回最后一次**真实上游 429** → 命中 fallback 白名单 → 备用集群被尝试 → 响应状态码：**200**（来自备用集群）；
- 两个 Key 均被写入 60s 惩罚键。

**请求 2**（60s 惩罚窗口内）：

- 响应状态码：**200**，body 为 `{"ok":true}`（来自备用集群）；
- 目标集群后端命中数 **与请求 1 后完全相同**（零新增——熔断，未发起任何目标集群请求）；
- 备用集群后端累计命中 2（请求 1 的 Key 级耗尽降级 + 请求 2 的熔断降级）；
- `bfe.log` 出现 `aiKeyAffinity: all usable keys penalized for cluster[cluster_session_affinity], circuit break` 与 `ServeHTTPForAI: fallback triggered, cluster[cluster_session_affinity] err[<nil>] status[429]`；
- exception.log 无新增异常；不出现 500。

> 注：请求 1 也返回 200（经降级）而非 429——这是 #1440 之前的既有语义（Key 级耗尽的最后响应交外层决策，429 命中白名单立即降级）。本 TC 区分两次请求的关键证据是**目标集群命中数**：请求 1 ≥2，请求 2 零新增（熔断生效）。

## 清理

停止 `bfe` 进程、Redis 与所有 mock 后端，删除临时目录。

## 备注

- 惩罚 TTL 为 60s（代码硬编码），集成测试**不等待过期**——"TTL 到期自动恢复"由 `filterPenaltyKeys` 的过滤逻辑保证，单测（`TestFilterPenaltyKeys_*`）已覆盖口径；若后续需要验证恢复，可在 Redis 中直接用短 TTL 手写惩罚键后 sleep 断言，本 TC 不做。
- 请求 1 中 `MaxRetries=3` 时实际尝试次数可能为 4（`usedSet` 全用过后重置再试），断言使用 `≥2` 而非精确值，避免与轮转实现细节耦合。
