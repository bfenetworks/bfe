# 简介

http_state 是HTTP1.0/1.1的状态信息。

# 监控项

| 监控项                       | 描述                                                         |
| ---------------------------- | ------------------------------------------------------------ |
| HTTP_BACKEND_CONN_ALL        | 连接后端的总数量                                             |
| HTTP_BACKEND_CONN_SUCC       | 连接后端的成功数量                                           |
| HTTP_BACKEND_REQ_ALL         | 发送请求到后端的总数量                                       |
| HTTP_BACKEND_REQ_SUCC        | 成功发送请求到后端的数量                                     |
| HTTP_CANCEL_ON_CLIENT_CLOSE  | 当服务端正在读后端响应时，如果客户端断连，取消该阻塞状态的数量 |
| HTTP_PANIC_BACKEND_READ      | 读后端panic的数量                                            |
| HTTP_PANIC_BACKEND_WRITE     | 写后端panic的数量                                            |
| HTTP_PANIC_CLIENT_FLUSH_LOOP | flushing循环出现panic的数量                                  |
| HTTP_PANIC_CLIENT_WATCH_LOOP | watching循环出现panic的数量                                  |

