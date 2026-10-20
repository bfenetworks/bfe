# TC-05 proxy_state SSE指标

## 用例编号与名称

TC-05 proxy_state SSE指标

## 所属场景

SC36 SSE 长连接语义与指标

## 版本声明

- `bfe`：当前源码版本

## 测试目的

验证请求侧与响应侧 SSE 各计数一次，普通请求不计数，结束后活跃计数为 0。

## 运行模式

单组件模式：仅启动真实 `bfe` 进程；后端为进程内 mock（HTTP/HTTPS/SSE 按用例配置）。

## 前置条件

1. 已编译 `bfe` 可执行文件。
2. mock 后端 `cluster_sse` 已启动；Redis（miniredis）已启动用于 token 规则加载。
3. `bfe.conf`：`EnableAiGateway=true`，加载 `mod_ai_route`、`mod_ai_token_auth`。
4. `ai_route.data` 中 `ak_auth` 命中 `cluster_sse`；`token_rule.data` 声明不限量 Key `ak_auth`。
5. `cluster_sse.ClusterBasic.TimeoutWriteClient=300`（毫秒）。

## 配置构造

- Arm 1：请求声明 SSE，后端立即返回一帧。
- Arm 2：普通 JSON 请求。
- Arm 3：不带 Accept，响应为 SSE 内容类型。

## BFE 请求

| 步骤 | Host | Path | 关键请求设置 | 说明 |
|------|------|------|--------------|------|
| 1 | sse.example.org | /v1/chat/completions | `Accept: text/event-stream` | 请求侧 SSE |
| 2 | sse.example.org | /v1/chat/completions | `Accept: application/json` | 普通 JSON |
| 3 | sse.example.org | /v1/chat/completions | 不带 `Accept` | 响应侧 SSE |

## 预期结果

- Arm 1 后 `SSE_REQ_SERVED=1`；Arm 2 后仍为 1；Arm 3 后为 2。
- 结束后 `SSE_REQ_ACTIVE=0`。

## 关联实现

- 测试代码：同场景目录下 `*_test.go` 的 `TestTC05_*` 函数
