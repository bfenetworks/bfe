# 简介

websocket_state 是websocket的状态信息。

# 监控项

| 监控项                        | 描述                        |
| ----------------------------- | --------------------------- |
| WEB_SOCKET_BYTES_RECV         | 收到websocket bytes的数量   |
| WEB_SOCKET_BYTES_SENT         | 发送websocket bytes的数量   |
| WEB_SOCKET_ERR_BACKEND_REJECT | 后端拒绝websocket请求的数量 |
| WEB_SOCKET_ERR_BALANCE        | 负载均衡出错的数量          |
| WEB_SOCKET_ERR_CONNECT        | 连接错误的数量              |
| WEB_SOCKET_ERR_HANDSHAKE      | 握手错误的数量              |
| WEB_SOCKET_ERR_PROXY          | 查找后端错误的数量          |
| WEB_SOCKET_ERR_TRANSFER       | 转发错误的数量              |
| WEB_SOCKET_PANIC_CONN         | 连接出现panic的数量         |

