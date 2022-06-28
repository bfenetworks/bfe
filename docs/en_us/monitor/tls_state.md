# TLS

## Introduction

The endpoint `/monitor/tls_state` exposes metrics about TLS protocol.

## Metrics

| Metric                                     | Description                                                  |
| ------------------------------------------ | ------------------------------------------------------------ |
| TLS_HANDSHAKE_ACCEPT_ECDHE_WITHOUT_EXT     | Counter for proposing ECDHE without extensions               |
| TLS_HANDSHAKE_ACCEPT_SSLV2_CLIENT_HELLO    | Counter for accepting SSLv2 client-hello                     |
| TLS_HANDSHAKE_CHECK_RESUME_SESSION_CACHE   | Counter for checking resume session cache                    |
| TLS_HANDSHAKE_CHECK_RESUME_SESSION_TICKET  | Counter for checking resume session ticket                   |
| TLS_HANDSHAKE_FULL_ALL                     | Counter for full TLS handshake                               |
| TLS_HANDSHAKE_FULL_SUCC                    | Counter for successful TLS handshake                         |
| TLS_HANDSHAKE_NO_SHARED_CIPHER_SUITE       | Counter for no cipher suite supported by both client and server |
| TLS_HANDSHAKE_OCSP_TIME_ERR                | Counter for ocsp time error                                  |
| TLS_HANDSHAKE_READ_CLIENT_HELLO_ERR        | Counter for reading client-hello error                       |
| TLS_HANDSHAKE_RESUME_ALL                   | Counter for resuming session                                 |
| TLS_HANDSHAKE_RESUME_SUCC                  | Counter for resuming session successfully                    |
| TLS_HANDSHAKE_SHOULD_RESUME_SESSION_CACHE  | Counter for resuming session by session cache                |
| TLS_HANDSHAKE_SHOULD_RESUME_SESSION_TICKET | Counter for resuming session by session ticket               |
| TLS_HANDSHAKE_SSLV2_NOT_SUPPORT            | Counter for unsupported SSLv2 handshake received             |
| TLS_HANDSHAKE_ZERO_DATA                    | Counter for zero data                                        |
| TLS_STATUS_REQUEST_EXT_COUNT               | Counter for request extensions                               |
