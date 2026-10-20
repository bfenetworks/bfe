# TC-02 TPM 超限拒绝

## 用例编号与名称

TC-02 TPM 超限拒绝

## 所属场景

SC34 `mod_ai_rate_limit` 分布式限流语义

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 TPM 规则按真实 usage 记账后，累计用量超过 `max_tokens` 时拒绝下一次请求。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；AI 后端为进程内 mock，Redis 为进程内 miniredis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动；mock 后端 `cluster_rl_main`、`cluster_rl_fallback` 已启动，返回固定 usage（prompt=100, completion=50, total=150）。
3. `bfe.conf` 中 `EnableAiGateway=true`，加载 `mod_ai_token_auth`、`mod_ai_route`、`mod_body_process`、`mod_ai_rate_limit`、`mod_access_pb3`。
4. `ai_route.data` 中 `ak_sc34` 命中 `cluster_rl_main`，fallback 为 `cluster_rl_fallback`。
5. `token_rule.data` 绑定 `ak_sc34` 到不限量配额计划，保证鉴权不干扰限流。

## 配置构造

- 限流策略 `rlp-sc34`：TPM 规则 `max_tokens=100`、`window_minutes=1`、`step_minutes=1`、`redis_key=default_bfe_sc34_tpm`。
- `mod_body_process` 加载默认规则以解析响应 usage。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body | 说明 |
|------|------|------|---------------|------|------|
| 1 | rl.example.org | /v1/chat/completions | Bearer ak_sc34 | {"model":"gpt-4"} | 消耗 150 token 的真实 usage |
| 2 | rl.example.org | /v1/chat/completions | Bearer ak_sc34 | {"model":"gpt-4"} | TPM 已超阈值 |

## 预期结果

- 步骤 1：HTTP 200；轮询 `default_bfe_sc34_tpm` 哈希累计值达到 150。
- 步骤 2：HTTP 429，`error.code=TPM_LIMIT_EXCEEDED`，`details.limit_type=tpm`。

## 说明

- 检查时预扣为 0（`ReservedX/ReservedOff` 恒为 0），因此拒绝发生在第一次请求完成记账之后。

## 关联实现

- 测试代码：见同场景目录下的 `*_test.go` 同名 `TestTC02_*` 函数
