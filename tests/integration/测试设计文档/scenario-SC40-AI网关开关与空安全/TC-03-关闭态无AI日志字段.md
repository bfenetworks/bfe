# TC-03 关闭态无 AI 日志字段

## 用例编号与名称

TC-03 关闭态无 AI 日志字段

## 所属场景

SC40 AI 网关开关与空安全

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证关闭态访问日志不输出 AI 字段。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；AI 后端为进程内 mock，Redis 为进程内 miniredis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动；mock 后端 `cluster_auth` 已启动。
3. `bfe.conf` 固定加载 `mod_ai_route`、`mod_ai_token_auth`、`mod_body_process`、`mod_access_pb3`；`EnableAiGateway` 由用例改写。
4. `ai_route.data` 中 `ak_auth` 命中 `cluster_auth`；`token_rule.data` 声明不限量 Key `ak_auth`。

## 配置构造

- `EnableAiGateway=false`；`mod_access_pb3` 正常输出请求日志。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body | 说明 |
|------|------|------|---------------|------|------|
| 1 | auth.example.org | /v1/chat/completions | （无） | {"model":"gpt-4"} | 关闭态请求 |

## 预期结果

- 访问日志 `AiMode`、`AiTargetModel`、`AiTotalTokens` 均未设置（nil）。

## 关联实现

- 测试代码：见同场景目录下的 `*_test.go` 同名 `TestTC03_*` 函数
