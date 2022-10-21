# WebSocket

## 简介

`/monitor/websocket_state`接口返回WebSocket相关指标。

## 监控项

| 监控项                        | 描述                                |
| ----------------------------- | ----------------------------------- |
| WEB_SOCKET_BYTES_RECV         | 接收字的总节数                      |
| WEB_SOCKET_BYTES_SENT         | 发送字的总节数                      |
| WEB_SOCKET_ERR_BACKEND_REJECT | 后端拒绝升级为WebSocket协议的错误数 |
| WEB_SOCKET_ERR_BALANCE        | 负载均衡失败的错误数                |
| WEB_SOCKET_ERR_CONNECT        | 连接后端失败的错误数                |
| WEB_SOCKET_ERR_HANDSHAKE      | WebSocket握手失败数                 |
| WEB_SOCKET_ERR_PROXY          | 无可用后端错误数                    |
| WEB_SOCKET_ERR_TRANSFER       | 数据传输的错误数                    |
| WEB_SOCKET_PANIC_CONN         | 连接PANIC的异常数                   |
