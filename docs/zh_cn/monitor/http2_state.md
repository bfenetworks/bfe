# HTTP2

## 简介

http2_state 是HTTP2的状态信息。

## 监控项

| 监控项                      | 描述                          |
| --------------------------- | ---------------------------- |
| H2_ERR_MAX_HEADER_LIST_SIZE | 请求header超限错误的数量       |
| H2_ERR_MAX_HEADER_URI_SIZE  | 请求中URI超限错误的数量       |
| H2_ERR_MAX_STREAM_PER_CONN  | 连接并发流超限错误的数量      |
| H2_ERR_GOT_RESET            | 流收到RESET帧的数量           |
| H2_PANIC_CONN               | 连接处理协程panic的数量       |
| H2_PANIC_STREAM             | 流处理协程panic的数量         |
| H2_TIMEOUT_CONN             | 连接超时的数量                 |
| H2_TIMEOUT_PREFACE          | 读客户端perface超时数量         |
| H2_TIMEOUT_SETTING          | 读客户端settings超时数量        |
| H2_TIMEOUT_READ_STREAM      | 流读超时数量               |
| H2_TIMEOUT_WRITE_STREAM     | 流写超时数量               |
| H2_CONN_OVERLOAD            | 过载状态拒绝连接的数量         |
| H2_REQ_OVERLOAD             | 过载状态拒绝请求的数量         |
| H2_REQ_HEADER_COMPRESS_SIZE | 压缩的请求头总字节数           |
| H2_REQ_HEADER_ORIGINAL_SIZE | 解压的请求头总字节数           |
| H2_RES_HEADER_COMPRESS_SIZE | 压缩的响应头总字节数           |
| H2_RES_HEADER_ORIGINAL_SIZE | 解压的响应头总字节数           |
