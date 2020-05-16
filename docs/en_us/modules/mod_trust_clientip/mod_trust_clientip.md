# mod_trust_clientip

## Introduction 

mod_trust_clientip checkes the client IP of incoming request/connnection against trusted ip dictionary. If matched, the request/connection is marked as trusted.

## Module Configuration

### Description
conf/mod_trust_clientip/mod_trust_clientip.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.DataPath | String<br>path of rule configuraiton |

### Example
```ini
[Basic]
DataPath = mod_trust_clientip/trust_client_ip.data
```

## Rule Configuraiton

### Description
  conf/mod_trust_clientip/trust_client_ip.data

| Config Item       | Type   | Description                                                     |
| ----------------- | ------ | --------------------------------------------------------------- |
| Version           | String | Verson of config file                                           |
| Config            | Object | trusted ip config |
| Config{k}         | Struct | label
| Config{v}         | String | A list of ip segments |
| Config{v}[]       | Object | A ip segment |
| Config{v}[].Begin | String | start ip address |
| Config{v}[].End   | String | end ip address |

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
| CONN_ADDR_INTERNAL           | ^PCounter for connection from internal               |
| CONN_ADDR_INTERNAL_NOT_TRUST | ^PCounter for connection from internal and not trust |
| CONN_TOTAL                   | ^PCounter for all connnetion checked                 |
| CONN_TRUST_CLIENTIP          | ^PCounter for connnection from trust address         |

