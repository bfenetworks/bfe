# TC-14 模块监控 Redis 错误计数 JSON 与 prometheus 一致性

## 用例编号与名称

TC-14 模块监控 Redis 错误计数 JSON 与 prometheus 一致性（issue #1499）

## 所属场景

SC20 AI 缓存精确匹配

## 版本声明

- `bfe`：当前源码版本（含 issue #1499 修复）

## 测试目的

验证缓存 Redis 故障时，`mod_ai_cache` 模块监控的两个出口给出**一致**的 Redis 错误/延迟计数：

- JSON 出口 `GET /monitor/mod_ai_cache` 的 `REDIS_ERR` / `LATENCY_MS`（module state 计数器）；
- prometheus 出口 `GET /monitor/mod_ai_cache.prometheus` 的 `redis_err` / `latency_ms`（rule table 计数器）。

修复前 5 个计数器键（`REDIS_ERR`、`EMBEDDING_ERR`、`LATENCY_MS`、`EMBEDDING_LATENCY_MS`、`VECTOR_LATENCY_MS`）只在 rule table 递增、module state 从不递增，JSON 恒为 0 而 prometheus 正常增长——运维从模块状态页会误判 Redis 健康，缓存全量失效长期不可见。

同时验证故障下业务 fail-open 不回归：响应 200、miss、请求到达上游、不写缓存键。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程与嵌入式 Redis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis（配额用）已启动；`mod_ai_cache` 的 Redis 通过独立 BNS 指向**关闭的端口**（`127.0.0.1:1`，连接被拒），与配额 Redis 隔离——测试框架 `BFEConfigBuilder.AiCacheRedisAddr` 为本场景新增该能力。
3. mock 后端已启动，返回固定答案。
4. `ak_cache` 绑定不限额配额计划。

## 配置构造

- `mod_ai_token_auth` / `mod_ai_rate_limit` 等共享 `redis_bns`（指向嵌入式 Redis，存活）；
- `mod_ai_cache` 单独使用 `redis_bns_ai_cache`（指向 `127.0.0.1:1`，死亡）——`name_conf.data` 同时映射两个 BNS。

## BFE 请求

发送 1 次 POST 请求：

| 字段 | 值 |
|------|-----|
| Host | `cache.example.org` |
| Path | `/v1/chat/completions` |
| Authorization | `Bearer ak_cache` |
| Body | 与 TC-01 相同的缓存问题 |

## 预期结果

- 业务 fail-open：响应 200、含上游答案、`X-Bfe-Ai-Cache` 非 hit、上游命中 1 次、Redis 中**无** `ai_cache:*` 缓存键写入；BFE 日志出现 redis get/setex failed（连接拒绝）。
- `GET /monitor/mod_ai_cache`：JSON `SCounters` 中 `REDIS_ERR ≥ 1`（修复前恒 0）、`REQ_TOTAL ≥ 1`、`CACHE_MISS ≥ 1`。
- `GET /monitor/mod_ai_cache.prometheus`：`redis_err` / `latency_ms` / `req_total` / `cache_miss` 可读。
- **一致性断言（本 TC 核心）**：`REDIS_ERR == redis_err`、`LATENCY_MS == latency_ms`、`REQ_TOTAL == req_total`、`CACHE_MISS == cache_miss`——同一指标在两个出口不得分叉。

## 清理

停止 `bfe` 进程、mock 后端与嵌入式 Redis，删除临时目录。
