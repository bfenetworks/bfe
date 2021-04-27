# SPDY

## 简介

`/monitor/spdy_state`接口返回SPDY相关指标。

## 监控项

| 监控项                         | 描述                    |
| ------------------------------ | ----------------------- |
| SPDY_ERR_BAD_REQUEST           | 错误请求数              |
| SPDY_ERR_FLOW_CONTROL          | 流量控制的错误数        |
| SPDY_ERR_GOT_RESET             | 收到RESET帧的次数       |
| SPDY_ERR_INVALID_DATA_STREAM   | 非法数据流的个数        |
| SPDY_ERR_INVALID_SYN_STREAM    | 非法SYN_STREAM的个数    |
| SPDY_ERR_MAX_STREAM_PER_CONN   | 连接并发流超限的错误数  |
| SPDY_ERR_NEW_FRAMER            | 新建Framer失败的错误数  |
| SPDY_PANIC_CONN                | 连接处理协程panic的次数 |
| SPDY_PANIC_STREAM              | 流处理协程panic的次数   |
| SPDY_REQ_HEADER_COMPRESS_SIZE  | 压缩前请求头的大小      |
| SPDY_REQ_HEADER_ORIGINAL_SIZE  | 压缩后请求头的大小      |
| SPDY_RES_HEADER_COMPRESS_SIZE  | 压缩的响应头总字节数    |
| SPDY_RES_HEADER_ORIGINAL_SIZE  | 原始的响应头总字节数    |
| SPDY_TIMEOUT_CONN              | 连接超时的次数          |
| SPDY_TIMEOUT_READ_STREAM       | 读流超时                |
| SPDY_TIMEOUT_WRITE_STREAM      | 写流超时                |
| SPDY_UNKNOWN_FRAME             | 未知类型frame数         |
