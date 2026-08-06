# mod_tcp_keepalive

## Introduction

mod_tcp_keepalive is used to set strategy of sending keepalive message in tcp connection.

In some situation, like smart watch, the device is sensitive to power consumption, it may be necessary to close the TCP-KeepAlive heartbeat message or increase the interval of sending TCP-KeepAlive heartbeat message. mod_tcp_keepalive can help to handle situation like this.

## Configuration

- [mod_tcp_keepalive.conf](../../configuration/mod_tcp_keepalive/mod_tcp_keepalive.conf.md)
- [tcp_keepalive.data](../../configuration/mod_tcp_keepalive/tcp_keepalive.data.md)

## Metrics

| Metric        | Description                         |
| ------------- | ---------------------------- |
| CONN_TO_SET    | Counter for connection which hit rule, to set or disable keeplaive                     |
| CONN_SET_KEEP_IDLE | Counter for connection set keepalive idle |
| CONN_SET_KEEP_IDLE_ERROR | Counter for connection set keepalive idle error |
| CONN_SET_KEEP_INTVL | Counter for connection set keepalive interval |
| CONN_SET_KEEP_INTVL_ERROR | Counter for connection set keepalive interval error |
| CONN_SET_KEEP_CNT | Counter for connection set keepalive retry count |
| CONN_SET_KEEP_CNT_ERROR | Counter for connection set keepalive retry count error |
| CONN_DISABLE_KEEP_ALIVE | Counter for connection disable keepalive message |
| CONN_DISABLE_KEEP_ALIVE_ERROR | Counter for connection disable keepalive error |
| CONN_CONVERT_TO_TCP_CONN_ERROR | Counter for connection convert to TCPConn error |
