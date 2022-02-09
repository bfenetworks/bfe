# 转发状态

## 简介

`/monitor/proxy_state`接口返回BFE服务核心状态指标。

## 监控项

### 基本情况

| 监控项                           | 描述                   |
| ------------------------------- | --------------------- |
| CLIENT_CONN_ACTIVE              | 活跃连接数              |
| CLIENT_CONN_SERVED              | 处理连接数              |
| CLIENT_CONN_UNFINISHED_REQ      | 存在未完成的请求的连接数   |
| CLIENT_CONN_USE100_CONTINUE     | 使用100-continue的连接数 |
| CLIENT_REQ_ACTIVE               | 活跃请求数               |
| CLIENT_REQ_SERVED               | 处理请求数               |
| CLIENT_REQ_FAIL                 | 转发失败的请求数          |

### 后端相关错误

| 监控项                           | 描述                   |
| ------------------------------- | --------------------- |
| ERR_BK_CONNECT_BACKEND          | 连接后端失败的错误数      |
| ERR_BK_FIND_PRODUCT             | 查找产品线失败的请求数    |
| ERR_BK_FIND_LOCATION            | 查找集群失败的请求数      |
| ERR_BK_NO_BALANCE               | 无负载均衡配置的请求数    |
| ERR_BK_NO_CLUSTER               | 无集群配置的请求数        |
| ERR_BK_READ_RESP_HEADER         | 读响应头失败的错误数      |
| ERR_BK_REQUEST_BACKEND          | 转发请求到后端失败的错误数 |
| ERR_BK_RESP_HEADER_TIMEOUT      | 读后端响应头超时的错误数   |
| ERR_BK_TRANSPORT_BROKEN         | 与后端连接异常的错误数     |
| ERR_BK_WRITE_REQUEST            | 向后端写请求失败的错误数   |

### 客户端相关错误

| 监控项                           | 描述                      |
| ------------------------------- | ------------------------ |
| ERR_CLIENT_BAD_REQUEST          | 客户端请求格式错误数         |
| ERR_CLIENT_CLOSE                | 客户端关闭连接错误数         |
| ERR_CLIENT_CONN_ACCEPT          | Accept客户端连接失败错误数   |
| ERR_CLIENT_EXPECT_FAIL          | 请求携带非法Except头部错误数 |
| ERR_CLIENT_LONG_HEADER          | 请求头部长度超限错误数       |
| ERR_CLIENT_LONG_URL             | 请求URL长度超限错误数        |
| ERR_CLIENT_RESET                | 客户端Reset连接错误数        |
| ERR_CLIENT_TIMEOUT              | 读客户端超时错误数           |
| ERR_CLIENT_WRITE                | 向客户端发送响应错误数        |
| ERR_CLIENT_ZERO_CONTENTLEN      | 对于100-continue请求，Content-Length为0错误数 |

### Panic相关异常

| 监控项                           | 描述                   |
| ------------------------------- | --------------------- |
| PANIC_BACKEND_READ              | 读后端协程panic的次数    |
| PANIC_BACKEND_WRITE             | 写后端协程panic的次数    |
| PANIC_CLIENT_CONN_SERVE         | 客户端连接协程panic的次数 |

### 流量相关

| 监控项                           | 描述               |
| ------------------------------- | ------------------ |
| HTTP2_CLIENT_CONN_ACTIVE        | HTTP2协议活跃连接数  |
| HTTP2_CLIENT_CONN_SERVED        | HTTP2协议处理连接数  |
| HTTP2_CLIENT_REQ_ACTIVE         | HTTP2协议活跃请求数  |
| HTTP2_CLIENT_REQ_SERVED         | HTTP2协议处理请求数  |
| HTTPS_CLIENT_CONN_ACTIVE        | HTTPS协议活跃连接数  |
| HTTPS_CLIENT_CONN_SERVED        | HTTPS协议处理连接数  |
| HTTPS_CLIENT_REQ_ACTIVE         | HTTPS协议活跃请求数  |
| HTTPS_CLIENT_REQ_SERVED         | HTTPS协议处理请求数  |
| HTTP_CLIENT_CONN_ACTIVE         | HTTP协议活跃连接数   |
| HTTP_CLIENT_CONN_SERVED         | HTTP协议处理连接数   |
| HTTP_CLIENT_REQ_ACTIVE          | HTTP协议活跃请求数   |
| HTTP_CLIENT_REQ_SERVED          | HTTP协议处理请求数   |
| SPDY_CLIENT_CONN_ACTIVE         | SPDY协议活跃连接数   |
| SPDY_CLIENT_CONN_SERVED         | SPDY协议处理连接数   |
| SPDY_CLIENT_REQ_ACTIVE          | SPDY协议活跃请求数   |
| SPDY_CLIENT_REQ_SERVED          | SPDY协议处理请求数   |
| STREAM_CLIENT_CONN_ACTIVE       | STREAM协议活跃连接数 |
| STREAM_CLIENT_CONN_SERVED       | STREAM协议处理连接数 |
| WSS_CLIENT_CONN_ACTIVE          | WSS协议活跃连接数    |
| WSS_CLIENT_CONN_SERVED          | WSS协议处理连接数    |
| WS_CLIENT_CONN_ACTIVE           | WS协议活跃连接数     |
| WS_CLIENT_CONN_SERVED           | WS协议处理连接数     |

### TLS协议相关

| 监控项                           | 描述                       |
| ------------------------------- | ------------------------- |
| SESSION_CACHE_CONN              | 与session cache建立的连接数  |
| SESSION_CACHE_CONN_FAIL         | 与session cache建连失败数    |
| SESSION_CACHE_GET               | 查询session cache请求数      |
| SESSION_CACHE_GET_FAIL          | 查询session cache失败请求数  |
| SESSION_CACHE_HIT               | 查询session cache命中请求数  |
| SESSION_CACHE_MISS              | 查询session cache未命中请求数 |
| SESSION_CACHE_SET               | 写入session cache请求数      |
| SESSION_CACHE_SET_FAIL          | 写入session cache失败请求数   |
| SESSION_CACHE_TYPE_NOT_BYTES    | 查询session cache值类型异常数 |
| TLS_HANDSHAKE_ALL               | TLS握手总数                  |
| TLS_HANDSHAKE_SUCC              | TLS握手成功数                |
