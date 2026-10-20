# TC-01 SSE请求不受写超时约束

## 用例编号与名称

TC-01 SSE请求不受写超时约束

## 所属场景

SC36 SSE 长连接语义与指标

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证声明 Accept: text/event-stream 的请求不受集群写超时限制，超时窗口之后写出的尾帧仍被送达。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock（HTTP/HTTPS/SSE 按用例配置）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_sse` 已启动；Redis（miniredis）已启动用于 token 规则加载。
3. `bfe.conf`：`EnableAiGateway=true`，加载 `mod_ai_route`、`mod_ai_token_auth`。
4. `ai_route.data` 中 `ak_auth` 命中 `cluster_sse`；`token_rule.data` 声明不限量 Key `ak_auth`。
5. `cluster_sse.ClusterBasic.TimeoutWriteClient=300`（毫秒）。

## 配置构造

- mock 后端：`SSEEvents=[首帧]`、`SSEHold` 阻塞 800ms、`SSETrailing=[尾帧]`。
- 集群写超时 300ms，小于首帧到尾帧的间隔。

## BFE 请求

| 步骤 | Host | Path | 关键请求设置 | 说明 |
|------|------|------|--------------|------|
| 1 | sse.example.org | /v1/chat/completions | `Accept: text/event-stream` | 首帧后阻塞 800ms 再发尾帧 |

## 预期结果

- HTTP 200；响应体同时包含首帧与尾帧内容（未被 300ms 写超时切断）。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC01_*` 函数
