# HTTP2

## 简介

`/monitor/http2_state`接口返回HTTP2相关指标。

## 监控项

| 监控项                      | 描述                      |
| --------------------------- | ----------------------- |
| H2_ERR_MAX_HEADER_LIST_SIZE | 请求Header大小超限的错误数  |
| H2_ERR_MAX_HEADER_URI_SIZE  | 请求中URI超限的错误数       |
| H2_ERR_MAX_STREAM_PER_CONN  | 连接并发流超限的错误数       |
| H2_ERR_GOT_RESET            | 收到RESET帧的次数          |
| H2_PANIC_CONN               | 连接处理协程panic的次数     |
| H2_PANIC_STREAM             | 流处理协程panic的次数       |
| H2_REQ_HEADER_COMPRESS_SIZE | 压缩的请求头总字节数         |
| H2_REQ_HEADER_ORIGINAL_SIZE | 原始的请求头总字节数         |
| H2_RES_HEADER_COMPRESS_SIZE | 压缩的响应头总字节数         |
| H2_RES_HEADER_ORIGINAL_SIZE | 原始的响应头总字节数         |
| H2_TIMEOUT_CONN             | 连接超时的次数              |
| H2_TIMEOUT_PREFACE          | 读客户端perface超时次数     |
| H2_TIMEOUT_SETTING          | 读客户端settings超时次数    |
| H2_TIMEOUT_READ_STREAM      | 流读超时次数               |
| H2_TIMEOUT_WRITE_STREAM     | 流写超时次数               |
