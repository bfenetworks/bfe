# 简介

spdy_state 是SPDY的状态信息。

# 监控项

| 监控项                         | 描述                             |
| ------------------------------ | -------------------------------- |
| SPDY_CONN_OVERLOAD             | 连接数超过负载的数量             |
| SPDY_ERR_BAD_REQUEST           | 错误请求数                       |
| SPDY_ERR_FLOW_CONTROL          | 流量控制的数量                   |
| SPDY_ERR_GOT_RESET             | 收到RST_STREAM的数量             |
| SPDY_ERR_INVALID_DATA_STREAM   | 非法数据流的个数                 |
| SPDY_ERR_INVALID_SYN_STREAM    | 非法SYN stream的个数             |
| SPDY_ERR_MAX_STREAM_PER_CONN   | 达到连接建议的最大stream数的数量 |
| SPDY_ERR_NEW_FRAMER            | 新建frame错误的数量              |
| SPDY_ERR_STREAM_ALREADY_CLOSED | stream早已关闭的数量             |
| SPDY_ERR_STREAM_CANCEL         | 主动关闭stream的数量             |
| SPDY_PANIC_CONN                | 连接出现panic的数量              |
| SPDY_PANIC_STREAM              | stream出现panic的数量            |
| SPDY_REQ_HEADER_COMPRESS_SIZE  | 压缩前，请求头的大小             |
| SPDY_REQ_HEADER_ORIGINAL_SIZE  | 压缩后，请求头的大小             |
| SPDY_REQ_OVERLOAD              | 请求数超过负载的数量             |
| SPDY_RES_HEADER_COMPRESS_SIZE  | 压缩前，响应头的大小             |
| SPDY_RES_HEADER_ORIGINAL_SIZE  | 压缩后，响应头的大小             |
| SPDY_TIMEOUT_CONN              | SPDY连接出现超时的数量           |
| SPDY_TIMEOUT_READ_STREAM       | 读stream超时                     |
| SPDY_TIMEOUT_WRITE_STREAM      | 写stream超时                     |
| SPDY_UNKNOWN_FRAME             | 未知frame的个数                  |