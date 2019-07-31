# 简介

stream_state 是 STREAM的状态信息。

# 监控项

| 监控项              | 描述                |
| ------------------- | ------------------- |
| STREAM_BYTES_RECV   | 收到stream的数量    |
| STREAM_BYTES_SENT   | 发送stream的数量    |
| STREAM_ERR_BALANCE  | 均衡出现错误的数量  |
| STREAM_ERR_CONNECT  | 连接后端的错误数    |
| STREAM_ERR_PROXY    | 查找后端的错误数    |
| STREAM_ERR_TRANSFER | 转发的错误数        |
| STREAM_PANIC_CONN   | 连接出现panic的数量 |