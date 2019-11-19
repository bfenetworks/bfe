# 简介

proxy_state 是BFE服务的核心状态信息。

# 监控项

## 基本情况

| 监控项                          | 描述                                |
| ------------------------------- | ----------------------------------- |
| CLIENT_CONN_ACTIVE              | 活跃连接数                          |
| CLIENT_CONN_SERVED              | 处理连接数                            |
| CLIENT_CONN_UNFINISHED_REQ      | 存在未完成的请求的连接数            |
| CLIENT_CONN_USE100_CONTINUE     | 使用100-continue的连接数            |
| CLIENT_REQ_ACTIVE               | 活跃请求数                          |
| CLIENT_REQ_SERVED               | 处理请求数                            |
| CLIENT_REQ_FAIL                 | 失败请求数                          |
| CLIENT_REQ_FAIL_WITH_NO_RETRY   | 无重试的失败请求数                  |
| CLIENT_REQ_WITH_CROSS_RETRY     | 跨集群重试的请求数                  |
| CLIENT_REQ_WITH_RETRY           | 重试的请求数                        |


## 客户端相关错误

| 监控项                          | 描述                                |
| ------------------------------- | ----------------------------------- |
| ERR_BK_CONNECT_BACKEND          | 连接后端错误的数量                    |
| ERR_BK_FIND_PRODUCT             | 查找产品线失败的数量                |
| ERR_BK_FIND_LOCATION            | 查找目的集群失败的数量                  |
| ERR_BK_NO_BALANCE               | 无负载均衡配置的数量                |
| ERR_BK_NO_CLUSTER               | 无集群配置的数量                    |
| ERR_BK_READ_RESP_HEADER         | 从后端读响应头错误的数量            |
| ERR_BK_REQUEST_BACKEND          | 转发请求到后端错误的数量            |
| ERR_BK_RESP_HEADER_TIMEOUT      | 从后端获取响应头超时的数量          |
| ERR_BK_TRANSPORT_BROKEN         | 后端连接出错数量                    |
| ERR_BK_WRITE_REQUEST            | 写请求到后端的错误数                |


## 后端相关错误

| 监控项                          | 描述                                |
| ------------------------------- | ----------------------------------- |
| ERR_CLIENT_BAD_REQUEST          | 客户端请求格式错误数                          |
| ERR_CLIENT_CLOSE                | 客户端关闭连接的数量                |
| ERR_CLIENT_CONN_ACCEPT          | 接受客户端连接失败的数量          |
| ERR_CLIENT_EXPECT_FAIL          | 客户端请求携带异常Except头部的数量            |
| ERR_CLIENT_LONG_HEADER          | 请求头长度超限的请求数                  |
| ERR_CLIENT_LONG_URL             | 请求URL长度超限的请求数                     |
| ERR_CLIENT_RESET                | 客户端Reset连接错误的数量                 |
| ERR_CLIENT_TIMEOUT              | 读客户端超时的数量                |
| ERR_CLIENT_WRITE                | 向客户端写响应错误的数量            |
| ERR_CLIENT_ZERO_CONTENTLEN      | 从客户端读取请求包含错误的零Content-Length的数量  |


## Panic相关异常

| 监控项                          | 描述                                |
| ------------------------------- | ----------------------------------- |
| PANIC_BACKEND_READ              | 读后端协程panic的数量               |
| PANIC_BACKEND_WRITE             | 写后端协程panic的数量               |
| PANIC_CLIENT_CONN_SERVE         | 客户端连接协程panic的数量     |


## 流量相关

| 监控项                          | 描述                                |
| ------------------------------- | ----------------------------------- |
| HTTP2_CLIENT_CONN_ACTIVE        | HTTP2协议活跃连接数               |
| HTTP2_CLIENT_CONN_SERVED        | HTTP2协议处理连接数                   |
| HTTP2_CLIENT_REQ_ACTIVE         | HTTP2协议活跃请求数               |
| HTTP2_CLIENT_REQ_SERVED         | HTTP2协议处理请求数                   |
| HTTPS_CLIENT_CONN_ACTIVE        | HTTPS协议活跃连接数               |
| HTTPS_CLIENT_CONN_SERVED        | HTTPS协议处理连接数                   |
| HTTPS_CLIENT_REQ_ACTIVE         | HTTPS协议活跃请求数               |
| HTTPS_CLIENT_REQ_SERVED         | HTTPS协议处理请求数                   |
| HTTP_CLIENT_CONN_ACTIVE         | HTTP协议活跃连接数         |
| HTTP_CLIENT_CONN_SERVED         | HTTP协议处理连接数             |
| HTTP_CLIENT_REQ_ACTIVE          | HTTP协议活跃请求数         |
| HTTP_CLIENT_REQ_SERVED          | HTTP协议处理请求数             |
| SPDY_CLIENT_CONN_ACTIVE         | SPDY协议活跃连接数                |
| SPDY_CLIENT_CONN_SERVED         | SPDY协议处理连接数                    |
| SPDY_CLIENT_REQ_ACTIVE          | SPDY协议活跃请求数                |
| SPDY_CLIENT_REQ_SERVED          | SPDY协议处理请求数                    |
| STREAM_CLIENT_CONN_ACTIVE       | STREAM协议活跃连接数              |
| STREAM_CLIENT_CONN_SERVED       | STREAM协议处理连接数                  |
| WSS_CLIENT_CONN_ACTIVE          | WSS协议活跃连接数                 |
| WSS_CLIENT_CONN_SERVED          | WSS协议处理连接数                     |
| WS_CLIENT_CONN_ACTIVE           | WS协议活跃连接数                  |
| WS_CLIENT_CONN_SERVED           | WS协议处理连接数                      |


## TLS协议相关

| 监控项                          | 描述                                |
| ------------------------------- | ----------------------------------- |
| SESSION_CACHE_CONN              | 与session cache建立连接的数量     |
| SESSION_CACHE_CONN_FAIL         | 与session cache建立连接失败的数量 |
| SESSION_CACHE_GET               | 查询session cache的数量             |
| SESSION_CACHE_GET_FAIL          | 查询session cache失败的数量         |
| SESSION_CACHE_HIT               | 查询session cache命中的数量             |
| SESSION_CACHE_MISS              | 查询session cache未命中的数量           |
| SESSION_CACHE_SET               | 写入session cache的数量             |
| SESSION_CACHE_SET_FAIL          | 写入session cache失败的数量         |
| SESSION_CACHE_TYPE_NOT_BYTES    | 查询session cache值类型异常的数量             |
| TLS_HANDSHAKE_ALL               | TLS握手总数量                         |
| TLS_HANDSHAKE_SUCC              | TLS握手成功数量                     |
| TLS_MULTI_CERT_CONN_VIP_UNKNOWN | 连接携带未知VIP的数量          |
| TLS_MULTI_CERT_CONN_WITHOUT_SNI | 连接未携带SNI的数量        |
| TLS_MULTI_CERT_CONN_WITHOUT_VIP | 连接未携带VIP的数量        |
| TLS_MULTI_CERT_USE_DEFAULT      | 连接使用默认TLS证书的数量               |
| TLS_MULTI_CERT_GET              | 查询TLS证书的数量                 |
| TLS_MULTI_CERT_UPDATE           | 更新TLS证书操作的数量                   |
| TLS_MULTI_CERT_UPDATE_ERR       | 更新TLS证书错误的数量               |
