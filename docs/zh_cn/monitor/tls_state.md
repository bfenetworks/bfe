# 简介

tls_state 是TLS的状态信息。

# 监控项

| 监控项                                     | 描述                                               |
| ------------------------------------------ | -------------------------------------------------- |
| TLS_HANDSHAKE_ACCEPT_ECDHE_WITHOUT_EXT     | 处理无扩展内容的ECDHE的数量                        |
| TLS_HANDSHAKE_ACCEPT_SSLV2_CLIENT_HELLO    | 收到SSLv2 client-hello的数量                       |
| TLS_HANDSHAKE_CHECK_RESUME_SESSION_CACHE   | 检查session cache，判定是否可以进行连接复用的数量  |
| TLS_HANDSHAKE_CHECK_RESUME_SESSION_TICKET  | 检查session ticket，判定是否可以进行连接复用的数量 |
| TLS_HANDSHAKE_FULL_ALL                     | 完全握手的数量                                     |
| TLS_HANDSHAKE_FULL_SUCC                    | 成功完成完全握手的数量                             |
| TLS_HANDSHAKE_NO_SHARED_CIPHER_SUITE       | 客户端和服务端均无加密套件的数量                   |
| TLS_HANDSHAKE_OCSP_TIME_ERR                | ocsp time错误的数量                                |
| TLS_HANDSHAKE_READ_CLIENT_HELLO_ERR        | 读client-hello错误的数量                           |
| TLS_HANDSHAKE_RESUME_ALL                   | 复用连接的数量                                     |
| TLS_HANDSHAKE_RESUME_SUCC                  | 复用连接成功的数量                                 |
| TLS_HANDSHAKE_SHOULD_RESUME_SESSION_CACHE  | 通过session cache复用连接的数量                    |
| TLS_HANDSHAKE_SHOULD_RESUME_SESSION_TICKET | 通过session ticket复用连接的数量                   |
| TLS_HANDSHAKE_SSLV2_NOT_SUPPORT            | 收到了不支持的SSLv2的握手请求数                    |
| TLS_HANDSHAKE_ZERO_DATA                    | 握手请求内容为空的数量                             |
| TLS_STATUS_REQUEST_EXT_COUNT               | 使用ocsp的数量                                     |

