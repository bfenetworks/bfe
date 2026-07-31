# Proxy

## Introduction

The endpoint `/monitor/proxy_state` exposes metrics about reverse proxy.

## Metrics

| Metric                          | Description                                              |
| ------------------------------- | -------------------------------------------------------- |
| CLIENT_CONN_ACTIVE              | Gauge for active connections                             |
| CLIENT_CONN_SERVED              | Counter for connections served                           |
| CLIENT_CONN_UNFINISHED_REQ      | Counter for connections closed with unfinished request   |
| CLIENT_CONN_USE100_CONTINUE     | Counter for connections using Expect 100 Continue        |
| CLIENT_REQ_ACTIVE               | Gauge for active requests                                |
| CLIENT_REQ_FAIL                 | Counter for failed requests                              |
| CLIENT_REQ_FAIL_WITH_NO_RETRY   | Counter for requests failed with no retry                |
| CLIENT_REQ_SERVED               | Counter for requests served                              |
| CLIENT_REQ_WITH_CROSS_RETRY     | Counter for requests served with cross cluster retry     |
| CLIENT_REQ_WITH_RETRY           | Counter for requests served with retry                   |
| ERR_BK_CONNECT_BACKEND          | Counter for connecting backend failed                    |
| ERR_BK_FIND_LOCATION            | Counter for finding cluster failed                       |
| ERR_BK_FIND_PRODUCT             | Counter for finding product failed                       |
| ERR_BK_NO_BALANCE               | Counter for no balance config of backend                 |
| ERR_BK_NO_CLUSTER               | Counter for no cluster config of backend                 |
| ERR_BK_BODY_PROCESS             | Counter for request/response body process error          |
| ERR_BK_READ_RESP_HEADER         | Counter for reading response header from backend failed  |
| ERR_BK_REQUEST_BACKEND          | Counter for invoking backend failed                      |
| ERR_BK_RESP_HEADER_TIMEOUT      | Counter for getting response header from backend timeout |
| ERR_BK_TRANSPORT_BROKEN         | Counter for transport broken of backend                  |
| ERR_BK_WRITE_REQUEST            | Counter for writing request to backend failed            |
| ERR_CLIENT_BAD_REQUEST          | Counter for bad request of client                        |
| ERR_CLIENT_CLOSE                | Counter for client closing connection                    |
| ERR_CLIENT_CONN_ACCEPT          | Counter for accepting connection from client failed      |
| ERR_CLIENT_EXPECT_FAIL          | Counter for expecting fail from client                   |
| ERR_CLIENT_LONG_HEADER          | Counter for request header too long                      |
| ERR_CLIENT_LONG_URL             | Counter for exceeding URI length limit                   |
| ERR_CLIENT_RESET                | Counter for resetting by client                          |
| ERR_CLIENT_TIMEOUT              | Counter for client accept or read timeout                |
| ERR_CLIENT_WRITE                | Counter for writing response to client failed            |
| ERR_CLIENT_ZERO_CONTENTLEN      | Counter for getting empty request content from client    |
| HTTP2_CLIENT_CONN_ACTIVE        | Gauge for active connections using HTTP2                 |
| HTTP2_CLIENT_CONN_SERVED        | Counter for connections served using HTTP2               |
| HTTP2_CLIENT_REQ_ACTIVE         | Gauge for active requests using HTTP2                    |
| HTTP2_CLIENT_REQ_SERVED         | Counter for requests served using HTTP2                  |
| HTTPS_CLIENT_CONN_ACTIVE        | Gauge for active connections using HTTPS                 |
| HTTPS_CLIENT_CONN_SERVED        | Counter for connections served using HTTPS               |
| HTTPS_CLIENT_REQ_ACTIVE         | Gauge for active requests using HTTPS                    |
| HTTPS_CLIENT_REQ_SERVED         | Counter for requests served using HTTPS                  |
| HTTP_CLIENT_CONN_ACTIVE         | Gauge for active connections using HTTP1.0/1.1           |
| HTTP_CLIENT_CONN_SERVED         | Counter for connections served using HTTP1.0/1.1         |
| HTTP_CLIENT_REQ_ACTIVE          | Gauge for active requests using HTTP1.0/1.1              |
| HTTP_CLIENT_REQ_SERVED          | Counter for requests served using HTTP1.0/1.1            |
| PANIC_BACKEND_READ              | Counter for reading from backend panic                   |
| PANIC_BACKEND_WRITE             | Counter for writing to backend panic                     |
| PANIC_CLIENT_CONN_SERVE         | Counter for accepting from client panic                  |
| SESSION_CACHE_CONN              | Counter for connection using session cache               |
| SESSION_CACHE_CONN_FAIL         | Counter for failed connection using session cache        |
| SESSION_CACHE_GET               | Counter for getting session cache                        |
| SESSION_CACHE_GET_FAIL          | Counter for getting session cache failed                 |
| SESSION_CACHE_HIT               | Counter for hitting session cache                        |
| SESSION_CACHE_MISS              | Counter for missing session cache                        |
| SESSION_CACHE_SET               | Counter for setting session cache                        |
| SESSION_CACHE_SET_FAIL          | Counter for setting session cache failed                 |
| SESSION_CACHE_TYPE_NOT_BYTES    | Counter for type of session cache is not bytes           |
| SESSION_CACHE_NO_INSTANCE       | Counter for no available session cache instance          |
| SPDY_CLIENT_CONN_ACTIVE         | Gauge for active connections using SPDY                  |
| SPDY_CLIENT_CONN_SERVED         | Counter for connections served using SPDY                |
| SPDY_CLIENT_REQ_ACTIVE          | Gauge for active requests using SPDY                     |
| SPDY_CLIENT_REQ_SERVED          | Counter for requests served using SPDY                   |
| STREAM_CLIENT_CONN_ACTIVE       | Gauge for active connections using STREAM                |
| STREAM_CLIENT_CONN_SERVED       | Counter for connections served using STREAM              |
| TLS_HANDSHAKE_ALL               | Counter for TLS handshake                                |
| TLS_HANDSHAKE_SUCC              | Counter for successful TLS handshake                     |
| TLS_MULTI_CERT_CONN_VIP_UNKNOWN | Counter for TLS cert lookup with unknown VIP             |
| TLS_MULTI_CERT_CONN_WITHOUT_SNI | Counter for TLS connections without SNI                  |
| TLS_MULTI_CERT_CONN_WITHOUT_VIP | Counter for TLS connections without VIP                  |
| TLS_MULTI_CERT_GET              | Counter for getting TLS cert                             |
| TLS_MULTI_CERT_UPDATE           | Counter for updating TLS cert                            |
| TLS_MULTI_CERT_UPDATE_ERR       | Counter for updating TLS cert failed                     |
| TLS_MULTI_CERT_USE_DEFAULT      | Counter for using default TLS cert                       |
| WSS_CLIENT_CONN_ACTIVE          | Gauge for active connections using WSS                   |
| WSS_CLIENT_CONN_SERVED          | Counter for connections served using WSS                 |
| WS_CLIENT_CONN_ACTIVE           | Gauge for active connections using WS                    |
| WS_CLIENT_CONN_SERVED           | Counter for connections served using WS                  |

### SSE

| Metric                          | Description                                              |
| ------------------------------- | -------------------------------------------------------- |
| SSE_REQ_SERVED                  | Counter for SSE requests served                          |
| SSE_REQ_ACTIVE                  | Gauge for active SSE requests                            |
