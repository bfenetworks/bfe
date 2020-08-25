# Proxy

## Introduction

The endpoint `/monitor/proxy_state` exposes metrics about reverse proxy.

## Metrics

| Metric                          | Description                                              |
| ------------------------------- | -------------------------------------------------------- |
| CLIENT_CONN_ACTIVE              | Counter for active connection                            |
| CLIENT_CONN_SERVED              | Counter for connection serverd                           |
| CLIENT_CONN_UNFINISHED_REQ      | Counter for connection closed with unfinished request    |
| CLIENT_CONN_USE100_CONTINUE     | Counter for connection used expect 100 continue          |
| CLIENT_REQ_ACTIVE               | Counter for active request                               |
| CLIENT_REQ_FAIL                 | Counter for failed request                               |
| CLIENT_REQ_FAIL_WITH_NO_RETRY   | Counter for request fail with no retry                   |
| CLIENT_REQ_SERVED               | Counter for request serverd                              |
| CLIENT_REQ_WITH_CROSS_RETRY     | Counter for request serverd with cross cluster retry     |
| CLIENT_REQ_WITH_RETRY           | Counter for request serverd with retry                   |
| ERR_BK_CONNECT_BACKEND          | Counter for connecting backend failed                    |
| ERR_BK_FIND_LOCATION            | Counter for finding location failed                      |
| ERR_BK_FIND_PRODUCT             | Counter for finding product failed                       |
| ERR_BK_NO_BALANCE               | Counter for no balance config of backend                 |
| ERR_BK_NO_CLUSTER               | Counter for no cluster config of backend                 |
| ERR_BK_READ_RESP_HEADER         | Counter for reading response header from backend failed  |
| ERR_BK_REQUEST_BACKEND          | Counter for fail in invoking backend                     |
| ERR_BK_RESP_HEADER_TIMEOUT      | Counter for getting response header from backend timeout |
| ERR_BK_TRANSPORT_BROKEN         | Counter for transport broken of backend                  |
| ERR_BK_WRITE_REQUEST            | Counter for writing request to backend failed            |
| ERR_CLIENT_BAD_REQUEST          | Counter for bad request of client                        |
| ERR_CLIENT_CLOSE                | Counter for client closing connection                    |
| ERR_CLIENT_CONN_ACCEPT          | Counter for accepting connection from client failed      |
| ERR_CLIENT_EXPECT_FAIL          | Counter for expecting fail from client                   |
| ERR_CLIENT_LONG_HEADER          | Counter for request entity too large                     |
| ERR_CLIENT_LONG_URL             | Counter for exceeding URI length limit                   |
| ERR_CLIENT_RESET                | Counter for reseting by client                           |
| ERR_CLIENT_TIMEOUT              | Counter for connecting with client timeout               |
| ERR_CLIENT_WRITE                | Counter for writing request to client failed             |
| ERR_CLIENT_ZERO_CONTENTLEN      | Counter for getting empty request content from client    |
| HTTP2_CLIENT_CONN_ACTIVE        | Counter for active connection using HTTP2                |
| HTTP2_CLIENT_CONN_SERVED        | Counter for connection serverd using HTTP2               |
| HTTP2_CLIENT_REQ_ACTIVE         | Counter for active request using HTTP2                   |
| HTTP2_CLIENT_REQ_SERVED         | Counter for request serverd using HTTP2                  |
| HTTPS_CLIENT_CONN_ACTIVE        | Counter for active connection using HTTPS                |
| HTTPS_CLIENT_CONN_SERVED        | Counter for connection serverd using HTTPS               |
| HTTPS_CLIENT_REQ_ACTIVE         | Counter for active request using HTTPS                   |
| HTTPS_CLIENT_REQ_SERVED         | Counter for request serverd using HTTPS                  |
| HTTP_CLIENT_CONN_ACTIVE         | Counter for active connection using HTTP1.0/1.1          |
| HTTP_CLIENT_CONN_SERVED         | Counter for connection serverd using HTTP1.0/1.1         |
| HTTP_CLIENT_REQ_ACTIVE          | Counter for active request using HTTP1.0/1.1             |
| HTTP_CLIENT_REQ_SERVED          | Counter for request serverd using HTTP1.0/1.1            |
| PANIC_BACKEND_READ              | Counter for reading from backend panic                   |
| PANIC_BACKEND_WRITE             | Counter for writing to backend panic                     |
| PANIC_CLIENT_CONN_SERVE         | Counter for accepting from client panic                  |
| SESSION_CACHE_CONN              | Counter for connection using session cache               |
| SESSION_CACHE_CONN_FAIL         | Counter for failed connection using session cache        |
| SESSION_CACHE_GET               | Counter for getting session cache                        |
| SESSION_CACHE_GET_FAIL          | Counter for getting session cache failed                 |
| SESSION_CACHE_HIT               | Counter for hittting session cache                       |
| SESSION_CACHE_MISS              | Counter for misssing session cache                       |
| SESSION_CACHE_SET               | Counter for setting session cache                        |
| SESSION_CACHE_SET_FAIL          | Counter for setting session cache failed                 |
| SESSION_CACHE_TYPE_NOT_BYTES    | Counter for type of session cache is not bytes           |
| SPDY_CLIENT_CONN_ACTIVE         | Counter for active connection using SPDY                 |
| SPDY_CLIENT_CONN_SERVED         | Counter for connection serverd using SPDY                |
| SPDY_CLIENT_REQ_ACTIVE          | Counter for active request using SPDY                    |
| SPDY_CLIENT_REQ_SERVED          | Counter for request serverd using SPDY                   |
| STREAM_CLIENT_CONN_ACTIVE       | Counter for active connection using STREAM               |
| STREAM_CLIENT_CONN_SERVED       | Counter for connection serverd using STREAM              |
| TLS_HANDSHAKE_ALL               | Counter for TLS handshake                                |
| TLS_HANDSHAKE_SUCC              | Counter for successful TLS handshake                     |
| TLS_MULTI_CERT_CONN_VIP_UNKNOWN | Counter for not getting TLS cert by vip                  |
| TLS_MULTI_CERT_CONN_WITHOUT_SNI | Counter for getting TLS cert not by SNI                  |
| TLS_MULTI_CERT_CONN_WITHOUT_VIP | Counter for getting TLS cert not by vip                  |
| TLS_MULTI_CERT_GET              | Counter for getting TLS cert                             |
| TLS_MULTI_CERT_UPDATE           | Counter for updating TLS cert                            |
| TLS_MULTI_CERT_UPDATE_ERR       | Counter for updating TLS cert failed                     |
| TLS_MULTI_CERT_USE_DEFAULT      | Counter for using TLS cert default                       |
| WSS_CLIENT_CONN_ACTIVE          | Counter for active connection using WSS                  |
| WSS_CLIENT_CONN_SERVED          | Counter for connection serverd using WSS                 |
| WS_CLIENT_CONN_ACTIVE           | Counter for active connection using WS                   |
| WS_CLIENT_CONN_SERVED           | Counter for connection serverd using WS                  |
