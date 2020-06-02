# TLS

## 简介

tls_state是处理TLS连接的状态信息。

## 监控项

| 监控项                                     | 描述                                               |
| ------------------------------------------ | -------------------------------------------------- |
| TLS_HANDSHAKE_ACCEPT_ECDHE_WITHOUT_EXT     | 处理无ECC扩展的ECDHE密钥交换的次数                      |
| TLS_HANDSHAKE_ACCEPT_SSLV2_CLIENT_HELLO    | 收到SSLv2版本client-hello的次数                      |
| TLS_HANDSHAKE_CHECK_RESUME_SESSION_CACHE   | 检查session cache，判定是否可以进行连接复用的次数  |
| TLS_HANDSHAKE_CHECK_RESUME_SESSION_TICKET  | 检查session ticket，判定是否可以进行连接复用的次数 |
| TLS_HANDSHAKE_FULL_ALL                     | 完全握手次数                                     |
| TLS_HANDSHAKE_FULL_SUCC                    | 完全握手成功的次数                             |
| TLS_HANDSHAKE_NO_SHARED_CIPHER_SUITE       | 客户端和服务端协商加密套件失败的次数                   |
| TLS_HANDSHAKE_OCSP_TIME_ERR                | OCSP stapling更新时间的错误数                         |
| TLS_HANDSHAKE_READ_CLIENT_HELLO_ERR        | 读client-hello失败的错误数                           |
| TLS_HANDSHAKE_RESUME_ALL                   | 简化握手次数                                     |
| TLS_HANDSHAKE_RESUME_SUCC                  | 简化握手成功的次数                                 |
| TLS_HANDSHAKE_SHOULD_RESUME_SESSION_CACHE  | 通过session cache进行简化握手的次数                    |
| TLS_HANDSHAKE_SHOULD_RESUME_SESSION_TICKET | 通过session ticket进行简化握手的次数                   |
| TLS_HANDSHAKE_SSLV2_NOT_SUPPORT            | 不支持SSLv2版本握手的次数                   |
| TLS_HANDSHAKE_ZERO_DATA                    | 读取到空client-hello的次数                             |
| TLS_STATUS_REQUEST_EXT_COUNT               | client-hello携带OCSP stapling的次数                   |

