# TC-07 多策略与未绑定 Key

## 用例编号与名称

TC-07 多策略与未绑定 Key

## 所属场景

SC34 `mod_ai_rate_limit` 分布式限流语义

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证多策略绑定（任一启用策略超限即拒）、禁用策略被跳过、未绑定限流策略的 Key 不受限。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；AI 后端为进程内 mock，Redis 为进程内 miniredis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动；mock 后端 `cluster_rl_main`、`cluster_rl_fallback` 已启动，返回固定 usage（prompt=100, completion=50, total=150）。
3. `bfe.conf` 中 `EnableAiGateway=true`，加载 `mod_ai_token_auth`、`mod_ai_route`、`mod_body_process`、`mod_ai_rate_limit`、`mod_access_pb3`。
4. `ai_route.data` 中 `ak_sc34` 命中 `cluster_rl_main`，fallback 为 `cluster_rl_fallback`。
5. `token_rule.data` 绑定 `ak_sc34` 到不限量配额计划，保证鉴权不干扰限流。

## 配置构造

- 策略 `rlp-sc34`：RPM=1，enabled。
- 策略 `rlp-sc34-1`：RPM=1，disabled。
- 绑定：`ak_sc34 -> [rlp-sc34, rlp-sc34-1]`；`ak_sc34_second -> [rlp-sc34-1]`；`ak_sc34_unbound` 无绑定。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body | 说明 |
|------|------|------|---------------|------|------|
| 1 | rl.example.org | /v1/chat/completions | Bearer ak_sc34 | {"model":"gpt-4"} | 主 Key 第 1 次 |
| 2 | rl.example.org | /v1/chat/completions | Bearer ak_sc34 | {"model":"gpt-4"} | 主 Key 第 2 次 |
| 3-4 | rl.example.org | /v1/chat/completions | Bearer ak_sc34_second | {"model":"gpt-4"} | 禁用策略 Key，两次 |
| 5-6 | rl.example.org | /v1/chat/completions | Bearer ak_sc34_unbound | {"model":"gpt-4"} | 未绑定 Key，两次 |

## 预期结果

- 步骤 1：200；步骤 2：429 `RPM_LIMIT_EXCEEDED`。
- 步骤 3-4：均 200（禁用策略被跳过）。
- 步骤 5-6：均 200（无策略绑定，不执行限流）。

## 关联实现

- 测试代码：见同场景目录下的 `*_test.go` 同名 `TestTC07_*` 函数
