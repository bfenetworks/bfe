# 简介

stream_state 是TLS-TCP反向代理的状态信息。

# 监控项

| 监控项              | 描述                |
| ------------------- | ------------------- |
| STREAM_BYTES_RECV   | 收到数据的总字节数 |
| STREAM_BYTES_SENT   | 发送数据的总字节数 |
| STREAM_ERR_BALANCE  | 无可用后端错误数 |
| STREAM_ERR_CONNECT  | 连接后端失败数 |
| STREAM_ERR_PROXY    | 由于后端集群异常拒绝用户连接数 |
| STREAM_ERR_TRANSFER | 数据传输的错误数 |
| STREAM_PANIC_CONN   | 连接panic的异常数 |
