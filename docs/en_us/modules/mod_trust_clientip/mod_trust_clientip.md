# mod_trust_clientip

## Introduction

mod_trust_clientip checks the client IP of incoming request/connnection against trusted ip dictionary. If matched, the request/connection is marked as trusted.

## Module Configuration

### Description

conf/mod_trust_clientip/mod_trust_clientip.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.DataPath | String<br>Path of rule configuration |

### Example

```ini
[Basic]
DataPath = mod_trust_clientip/trust_client_ip.data
```

## Rule Configuration

### Description

  conf/mod_trust_clientip/trust_client_ip.data

| Config Item       | Type   | Description                                                     |
| ----------------- | ------ | --------------------------------------------------------------- |
| Version           | String | Version of config file                                           |
| Config            | Object | Trusted ip config |
| Config{k}         | Struct | Label
| Config{v}         | String | A list of ip segments |
| Config{v}[]       | Object | A ip segment |
| Config{v}[].Begin | String | Start ip address |
| Config{v}[].End   | String | End ip address |

### Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "inner-idc": [
            {
                "Begin": "10.0.0.0",
                "End": "10.255.255.255"
            }
        ]
    }
}
```

## Metrics

| Metric                       | Description                                        |
| ---------------------------- | -------------------------------------------------- |
| CONN_ADDR_INTERNAL           | Counter for connection from internal               |
| CONN_ADDR_INTERNAL_NOT_TRUST | Counter for connection from internal and not trust |
| CONN_TOTAL                   | Counter for all connnetion checked                 |
| CONN_TRUST_CLIENTIP          | Counter for connection from trust address         |
