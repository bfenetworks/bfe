# Stream

## 简介

`/monitor/stream_state`接口返回TLS-TCP反向代理相关指标。

## 监控项

| 监控项              | 描述                 |
| ------------------- | -------------------- |
| STREAM_BYTES_RECV   | 收到的总字节数       |
| STREAM_BYTES_SENT   | 发送的总字节数       |
| STREAM_ERR_BALANCE  | 负载均衡失败的错误数 |
| STREAM_ERR_CONNECT  | 连接后端失败的错误数 |
| STREAM_ERR_PROXY    | 无可用后端错误数     |
| STREAM_ERR_TRANSFER | 数据传输的错误数     |
| STREAM_PANIC_CONN   | 连接panic的异常数    |
