# mod_tcp_keepalive

## Introduction

mod_tcp_keepalive is used to set strategy of sending keepalive message in tcp connection.

In some situation, like smart watch, the device is sensitive to power consumption, it may be necessary to close the TCP-KeepAlive heartbeat message or increase the interval of sending TCP-KeepAlive heartbeat message. mod_tcp_keepalive can help to handle situation like this.

## Module Configuration

### Description

conf/mod_tcp_keepalive/mod_tcp_keepalive.conf

| Config Item | Description |
| ----- | --- |
| Basic.DataPath | String<br>Path of product rule configuration |
| Log.OpenDebug | Boolean<br>Open debug mode or not |

### Example

```ini
[Basic]
DataPath = ../data/mod_tcp_keepalive/tcp_keepalive.data

[Log]
OpenDebug = false
```

## Rule Configuration

### Description

| Config Item | Description |
| ----- | --- |
| Version | String<br>Version of config file |
| Config | Object<br>Rules for each product |
| Config{k} | String<br>Product name |
| Config{v} | Array<br>A list of rules |
| Config{v}[] | Object<br>A specific rule |
| Config{v}[].VipConf | Array<br>The list of virtual IPs to set the keepalive message strategy  |
| Config{v}[].KeepAliveParam | Object<br>The specific keepalive message strategy|
| Config{v}[].KeepaliveParam.Disable | Bool<br>Disable sending keepalive message or not, default false |
| Config{v}[].KeepaliveParam.KeepIdle | Int<br>Period to send heartbeat message since there is no data transport in tcp connection |
| Config{v}[].KeepaliveParam.KeepIntvl | Int<br>Period to send heartbeat message again when last message is not applied |
| Config{v}[].KeepaliveParam.KeepCnt | Int<br>Counter to resend heartbeat message when last message is not applied |

### Example

```json
{
    "Config": {
        "product1": [
            {
                "VipConf": ["10.1.1.1", "10.1.1.2"],
                "KeepAliveParam": {
                    "KeepIdle": 70,
                    "KeepIntvl": 15,
                    "KeepCnt": 9
                }
            },
            {
                "VipConf": ["10.1.1.3"],
                "KeepAliveParam": {
                    "Disable": true
                }
            }
        ],
        "product2": [
            {
                "VipConf": ["10.2.1.1"],
                "KeepAliveParam": {
                    "KeepIdle": 20,
                    "KeepIntvl": 15
                }
            }
        ]
    },
    "Version": "2021-06-25 14:31:05"
}
```

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
| CONN_COVERT_TO_TCP_CONN_ERROR | Counter for connection convert to TCPConn error |
