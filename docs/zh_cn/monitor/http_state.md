# HTTP

## 简介

http_state 是HTTP1.0/1.1的状态信息。

## 监控项

| 监控项                        | 描述                                                     |
| ---------------------------- | ------------------------------------------------------- |
| HTTP_BACKEND_CONN_ALL        | 与后端建立连接的总数量                                      |
| HTTP_BACKEND_CONN_SUCC       | 与后端建立连接成功数量                                      |
| HTTP_BACKEND_REQ_ALL         | 转发请求到后端的总数量                                      |
| HTTP_BACKEND_REQ_SUCC        | 转发请求到后端成功数量                                      |
| HTTP_CANCEL_ON_CLIENT_CLOSE  | 由于客户端断连停止阻塞读后端响应事件数量                       |
| HTTP_PANIC_BACKEND_READ      | 后端READ协程panic的数量                                    |
| HTTP_PANIC_BACKEND_WRITE     | 后端WRITE协程panic的数量                                   |
| HTTP_PANIC_CLIENT_FLUSH_LOOP | 客户端FLUSH协程panic的数量                                 |
| HTTP_PANIC_CLIENT_WATCH_LOOP | 客户端WATCH协程panic的数量                                 |
