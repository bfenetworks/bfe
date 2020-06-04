# HTTP

## 简介

`/monitor/http_state`接口返回HTTP相关指标

## 监控项

| 监控项                       | 描述                       |
| ---------------------------- | -------------------------- |
| HTTP_BACKEND_CONN_ALL        | 与后端建立的总连接数       |
| HTTP_BACKEND_CONN_SUCC       | 与后端建立成功的连接数     |
| HTTP_BACKEND_REQ_ALL         | 转发到后端的总请求数       |
| HTTP_BACKEND_REQ_SUCC        | 成功转发到后端的请求数     |
| HTTP_PANIC_BACKEND_READ      | 后端READ协程panic的次数    |
| HTTP_PANIC_BACKEND_WRITE     | 后端WRITE协程panic的次数   |
| HTTP_PANIC_CLIENT_FLUSH_LOOP | 客户端FLUSH协程panic的次数 |
| HTTP_PANIC_CLIENT_WATCH_LOOP | 客户端WATCH协程panic的次数 |
