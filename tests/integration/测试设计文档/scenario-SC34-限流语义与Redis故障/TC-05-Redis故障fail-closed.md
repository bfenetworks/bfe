# TC-05 Redis 故障 fail-closed

## 用例编号与名称

TC-05 Redis 故障 fail-closed

## 所属场景

SC34 `mod_ai_rate_limit` 分布式限流语义

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 `IsRejectOnRedisError=true` 时 Redis 故障拒绝请求，并在访问日志中记录 `redis_error` 命中。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；AI 后端为进程内 mock，Redis 为进程内 miniredis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动；mock 后端 `cluster_rl_main`、`cluster_rl_fallback` 已启动，返回固定 usage（prompt=100, completion=50, total=150）。
3. `bfe.conf` 中 `EnableAiGateway=true`，加载 `mod_ai_token_auth`、`mod_ai_route`、`mod_body_process`、`mod_ai_rate_limit`、`mod_access_pb3`。
4. `ai_route.data` 中 `ak_sc34` 命中 `cluster_rl_main`，fallback 为 `cluster_rl_fallback`。
5. `token_rule.data` 绑定 `ak_sc34` 到不限量配额计划，保证鉴权不干扰限流。

## 配置构造

- `mod_ai_rate_limit.conf`：`IsRejectOnRedisError = true`（模板默认值）。
- 限流策略：RPM `max_requests=100`。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body | 说明 |
|------|------|------|---------------|------|------|
| 1 | rl.example.org | /v1/chat/completions | Bearer ak_sc34 | {"model":"gpt-4"} | Redis 正常 |
| 2 | rl.example.org | /v1/chat/completions | Bearer ak_sc34 | {"model":"gpt-4"} | 关闭 miniredis 后 |

## 预期结果

- 步骤 1：HTTP 200。
- 步骤 2：HTTP 500，`error.code=RATE_LIMIT_REDIS_ERROR`；后端命中数保持 1（未转发）。
- 停止 BFE 后解析访问日志：最后一条 `RequestLog.AiRateLimitHits` 含 `rate_limit_policy_id=rlp-sc34`、`rate_limit_type=redis_error`、`rule_names=[acc_redis_error]`。

## 说明

- 日志经 log4go 落盘，停止进程前留 500ms 刷新时间。

## 关联实现

- 测试代码：见同场景目录下的 `*_test.go` 同名 `TestTC05_*` 函数
