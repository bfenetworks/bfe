# 简介

http2_state 是HTTP2的状态信息。

# 监控项

| 监控项                      | 描述                             |
| --------------------------- | -------------------------------- |
| H2_CONN_OVERLOAD            | 连接数超过负载的数量             |
| H2_ERR_GOT_RESET            | 收到RST_STREAM的数量             |
| H2_ERR_MAX_HEADER_LIST_SIZE | 达到header表最大长度的数量       |
| H2_ERR_MAX_HEADER_URI_SIZE  | 达到header中URI的最大长度的数量  |
| H2_ERR_MAX_STREAM_PER_CONN  | 达到连接建议的最大stream数的数量 |
| H2_PANIC_CONN               | 连接出现panic的数量              |
| H2_PANIC_STREAM             | stream出现panic的数量            |
| H2_REQ_HEADER_COMPRESS_SIZE | 压缩后，请求头的大小             |
| H2_REQ_HEADER_ORIGINAL_SIZE | 压缩前，请求头的大小             |
| H2_REQ_OVERLOAD             | 请求数超过负载的数量             |
| H2_RES_HEADER_COMPRESS_SIZE | 压缩后，响应头的大小             |
| H2_RES_HEADER_ORIGINAL_SIZE | 压缩前，响应头的大小             |
| H2_TIMEOUT_CONN             | 连接超时的数量                   |
| H2_TIMEOUT_PREFACE          | 等待客户端perface超时数量        |
| H2_TIMEOUT_READ_STREAM      | 读stream超时数量                 |
| H2_TIMEOUT_SETTING          | 设置frames超时数量               |
| H2_TIMEOUT_WRITE_STREAM     | 写stream超时数量                 |

