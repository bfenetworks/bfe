# TC-06 Redis 故障 fail-open

## 用例编号与名称

TC-06 Redis 故障 fail-open

## 所属场景

SC34 `mod_ai_rate_limit` 分布式限流语义

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 `IsRejectOnRedisError=false` 时 Redis 故障不阻断业务请求。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；AI 后端为进程内 mock，Redis 为进程内 miniredis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动；mock 后端 `cluster_rl_main`、`cluster_rl_fallback` 已启动，返回固定 usage（prompt=100, completion=50, total=150）。
3. `bfe.conf` 中 `EnableAiGateway=true`，加载 `mod_ai_token_auth`、`mod_ai_route`、`mod_body_process`、`mod_ai_rate_limit`、`mod_access_pb3`。
4. `ai_route.data` 中 `ak_sc34` 命中 `cluster_rl_main`，fallback 为 `cluster_rl_fallback`。
5. `token_rule.data` 绑定 `ak_sc34` 到不限量配额计划，保证鉴权不干扰限流。

## 配置构造

- 生成配置后用 `common.RewriteConfLine` 将 `mod_ai_rate_limit.conf` 的 `IsRejectOnRedisError` 改为 `false`。
- 限流策略：RPM `max_requests=100`。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body | 说明 |
|------|------|------|---------------|------|------|
| 1 | rl.example.org | /v1/chat/completions | Bearer ak_sc34 | {"model":"gpt-4"} | 关闭 miniredis 后 |

## 预期结果

- HTTP 200，后端命中 1 次（fail-open 放行）。

## 关联实现

- 测试代码：见同场景目录下的 `*_test.go` 同名 `TestTC06_*` 函数
