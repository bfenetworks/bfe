# TC-01 RPM 滑动窗口边界与恢复

## 用例编号与名称

TC-01 RPM 滑动窗口边界与恢复

## 所属场景

SC34 `mod_ai_rate_limit` 分布式限流语义

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 RPM 规则按滑动窗口计数：窗口内超过 `max_requests` 时拒绝，窗口滑过后恢复放行，旧窗口成员被裁剪。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；AI 后端为进程内 mock，Redis 为进程内 miniredis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动；mock 后端 `cluster_rl_main`、`cluster_rl_fallback` 已启动，返回固定 usage（prompt=100, completion=50, total=150）。
3. `bfe.conf` 中 `EnableAiGateway=true`，加载 `mod_ai_token_auth`、`mod_ai_route`、`mod_body_process`、`mod_ai_rate_limit`、`mod_access_pb3`。
4. `ai_route.data` 中 `ak_sc34` 命中 `cluster_rl_main`，fallback 为 `cluster_rl_fallback`。
5. `token_rule.data` 绑定 `ak_sc34` 到不限量配额计划，保证鉴权不干扰限流。

## 配置构造

- `cluster_rl_main.AIConf.Keys`：单 Key（`sk-sc34-1`）。
- 限流策略 `rlp-sc34`：RPM 规则 `max_requests=2`、`window_minutes=1`、`redis_key=default_bfe_sc34_rpm`。
- 绑定关系：`ak_sc34 -> [rlp-sc34]`。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body | 说明 |
|------|------|------|---------------|------|------|
| 1 | rl.example.org | /v1/chat/completions | Bearer ak_sc34 | {"model":"gpt-4"} | 窗口内第 1 次 |
| 2 | rl.example.org | /v1/chat/completions | Bearer ak_sc34 | {"model":"gpt-4"} | 窗口内第 2 次 |
| 3 | rl.example.org | /v1/chat/completions | Bearer ak_sc34 | {"model":"gpt-4"} | 窗口内第 3 次 |
| 4 | rl.example.org | /v1/chat/completions | Bearer ak_sc34 | {"model":"gpt-4"} | Redis 虚拟时钟推进 90s 后 |

## 预期结果

- 步骤 1、2：HTTP 200，后端命中 2 次；Redis ZSET `default_bfe_sc34_rpm` 有 2 个成员。
- 步骤 3：HTTP 429，`error.code=RPM_LIMIT_EXCEEDED`，后端命中数不增加。
- 步骤 4：HTTP 200，响应成功；`ZMembers` 只剩 1 个新成员（旧窗口被 `ZREMRANGEBYSCORE` 裁剪）。

## 说明

- 时间推进使用 `common.RedisServer.AdvanceTime`：它同时推进 Redis `TIME`（滑动窗口依据）与 key TTL；仅 `FastForward` 只影响 TTL。
- 断言使用 90s 余量，避免用例执行耗时稀释 60s 窗口。

## 关联实现

- 测试代码：见同场景目录下的 `*_test.go` 同名 `TestTC01_*` 函数
