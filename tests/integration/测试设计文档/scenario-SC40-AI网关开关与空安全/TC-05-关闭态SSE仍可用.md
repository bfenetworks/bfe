# TC-05 关闭态 SSE 仍可用

## 用例编号与名称

TC-05 关闭态 SSE 仍可用

## 所属场景

SC40 AI 网关开关与空安全

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证 SSE 识别位于 `EnableAiGateway` 开关之外，关闭态流式响应仍正常透传并计入 `proxy_state` 指标。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；AI 后端为进程内 mock，Redis 为进程内 miniredis。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. 嵌入式 Redis 已启动；mock 后端 `cluster_auth` 已启动。
3. `bfe.conf` 固定加载 `mod_ai_route`、`mod_ai_token_auth`、`mod_body_process`、`mod_access_pb3`；`EnableAiGateway` 由用例改写。
4. `ai_route.data` 中 `ak_auth` 命中 `cluster_auth`；`token_rule.data` 声明不限量 Key `ak_auth`。

## 配置构造

- `EnableAiGateway=false`；mock 后端切换为 `SSEEvents` 模式（`Content-Type: text/event-stream`）。

## BFE 请求

| 步骤 | Host | Path | Authorization | Body | 说明 |
|------|------|------|---------------|------|------|
| 1 | auth.example.org | /v1/chat/completions | （无） | {"model":"gpt-4"} | `Accept: text/event-stream` |

## 预期结果

- HTTP 200，`Content-Type` 含 `text/event-stream`，响应体包含流式事件内容。
- `/monitor/proxy_state` 的 `SSE_REQ_SERVED` ≥ 1。

## 关联实现

- 测试代码：见同场景目录下的 `*_test.go` 同名 `TestTC05_*` 函数
