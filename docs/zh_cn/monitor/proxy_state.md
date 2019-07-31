# 简介

proxy_state 是BFE服务的核心状态信息。

# 简介

| 监控项                          | 描述                                |
| ------------------------------- | ----------------------------------- |
| CLIENT_CONN_ACTIVE              | 活跃连接数                          |
| CLIENT_CONN_SERVED              | 总连接数                            |
| CLIENT_CONN_UNFINISHED_REQ      | 存在未完成的请求的连接数            |
| CLIENT_CONN_USE100_CONTINUE     | 使用100-continue的连接数            |
| CLIENT_REQ_ACTIVE               | 活跃请求数                          |
| CLIENT_REQ_FAIL                 | 失败请求数                          |
| CLIENT_REQ_FAIL_WITH_NO_RETRY   | 未进行重试的失败请求数              |
| CLIENT_REQ_SERVED               | 总请求数                            |
| CLIENT_REQ_WITH_CROSS_RETRY     | 跨集群重试的请求数                  |
| CLIENT_REQ_WITH_RETRY           | 重试的请求数                        |
| ERR_BK_CONNECT_BACKEND          | 连接后端的错误数                    |
| ERR_BK_FIND_LOCATION            | 查找集群失败的数量                  |
| ERR_BK_FIND_PRODUCT             | 查找产品线失败的数量                |
| ERR_BK_NO_BALANCE               | 无负载均衡配置的数量                |
| ERR_BK_NO_CLUSTER               | 无集群配置的数量                    |
| ERR_BK_READ_RESP_HEADER         | 从后端读响应头错误的数量            |
| ERR_BK_REQUEST_BACKEND          | 转发请求到后端错误的数量            |
| ERR_BK_RESP_HEADER_TIMEOUT      | 从后端获取响应头超时的数量          |
| ERR_BK_TRANSPORT_BROKEN         | 后端连接出错数量                    |
| ERR_BK_WRITE_REQUEST            | 写请求到后端的错误数                |
| ERR_CLIENT_BAD_REQUEST          | 错误请求数                          |
| ERR_CLIENT_CLOSE                | 客户端关闭连接的数量                |
| ERR_CLIENT_CONN_ACCEPT          | 与客户端建立连接失败的数量          |
| ERR_CLIENT_EXPECT_FAIL          | 带有非法Except头的请求数            |
| ERR_CLIENT_LONG_HEADER          | 请求头太长的请求数                  |
| ERR_CLIENT_LONG_URL             | URL太长的请求数                     |
| ERR_CLIENT_RESET                | 端上发送reset的数量                 |
| ERR_CLIENT_TIMEOUT              | 连接客户端超时的数量                |
| ERR_CLIENT_WRITE                | 写响应到客户端失败的数量            |
| ERR_CLIENT_ZERO_CONTENTLEN      | 收到空请求的数量                    |
| HTTP2_CLIENT_CONN_ACTIVE        | 使用HTTP2的活跃连接数               |
| HTTP2_CLIENT_CONN_SERVED        | 使用HTTP2的连接数                   |
| HTTP2_CLIENT_REQ_ACTIVE         | 使用HTTP2的活跃请求数               |
| HTTP2_CLIENT_REQ_SERVED         | 使用HTTP2的请求数                   |
| HTTPS_CLIENT_CONN_ACTIVE        | 使用HTTPS的活跃连接数               |
| HTTPS_CLIENT_CONN_SERVED        | 使用HTTPS的连接数                   |
| HTTPS_CLIENT_REQ_ACTIVE         | 使用HTTPS的活跃请求数               |
| HTTPS_CLIENT_REQ_SERVED         | 使用HTTPS的请求数                   |
| HTTP_CLIENT_CONN_ACTIVE         | 使用HTTP1.0/1.1的活跃连接数         |
| HTTP_CLIENT_CONN_SERVED         | 使用HTTP1.0/1.1的连接数             |
| HTTP_CLIENT_REQ_ACTIVE          | 使用HTTP1.0/1.1的活跃请求数         |
| HTTP_CLIENT_REQ_SERVED          | 使用HTTP1.0/1.1的请求数             |
| PANIC_BACKEND_READ              | 读后端出现panic的数量               |
| PANIC_BACKEND_WRITE             | 写后端出现panic的数量               |
| PANIC_CLIENT_CONN_SERVE         | 与客户端建立连接出现panic的数量     |
| SESSION_CACHE_CONN              | 使用session cache建立连接的数量     |
| SESSION_CACHE_CONN_FAIL         | 使用session cache建立连接失败的数量 |
| SESSION_CACHE_GET               | 获取session cache的数量             |
| SESSION_CACHE_GET_FAIL          | 获取session cache失败的数量         |
| SESSION_CACHE_HIT               | 命中session cache的数量             |
| SESSION_CACHE_MISS              | 未命中session cache的数量           |
| SESSION_CACHE_SET               | 设置session cache的数量             |
| SESSION_CACHE_SET_FAIL          | 设置session cache失败的数量         |
| SESSION_CACHE_TYPE_NOT_BYTES    | 会话类型不是bytes的数量             |
| SPDY_CLIENT_CONN_ACTIVE         | 使用SPDY的活跃连接数                |
| SPDY_CLIENT_CONN_SERVED         | 使用SPDY的连接数                    |
| SPDY_CLIENT_REQ_ACTIVE          | 使用SPDY的活跃请求数                |
| SPDY_CLIENT_REQ_SERVED          | 使用SPDY的请求数                    |
| STREAM_CLIENT_CONN_ACTIVE       | 使用STREAM的活跃连接数              |
| STREAM_CLIENT_CONN_SERVED       | 使用STREAM的连接数                  |
| TLS_HANDSHAKE_ALL               | TLS握手数量                         |
| TLS_HANDSHAKE_SUCC              | TLS握手成功数量                     |
| TLS_MULTI_CERT_CONN_VIP_UNKNOWN | 通过vip未找到TLS证书的数量          |
| TLS_MULTI_CERT_CONN_WITHOUT_SNI | 不是通过SNI获取TLS证书的数量        |
| TLS_MULTI_CERT_CONN_WITHOUT_VIP | 不是通过vip获取TLS证书的数量        |
| TLS_MULTI_CERT_GET              | 获取到TLS证书的数量                 |
| TLS_MULTI_CERT_UPDATE           | 更新TLS证书的数量                   |
| TLS_MULTI_CERT_UPDATE_ERR       | 更新TLS证书错误的数量               |
| TLS_MULTI_CERT_USE_DEFAULT      | 使用默认TLS证书的数量               |
| WSS_CLIENT_CONN_ACTIVE          | 使用WSS的活跃连接数                 |
| WSS_CLIENT_CONN_SERVED          | 使用WSS的连接数                     |
| WS_CLIENT_CONN_ACTIVE           | 使用WS的活跃连接数                  |
| WS_CLIENT_CONN_SERVED           | 使用WS的连接数                      |