# mod_tcp_keepalive Rule Configuration

## Configuration Introduction

`tcp_keepalive.data` is the rule configuration file for the `mod_tcp_keepalive` module, used to configure TCP keepalive message strategies for each product (tenant).

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | - | Type is [Version](../00-common.md#5-version) |
| Config | Object | Rules for each product | Y | - | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | A list of rules | Y | - | - |
| Config{v}[] | Object | A specific rule | Y | - | - |
| Config{v}[].VipConf | Array | The list of virtual IPs to set the keepalive message strategy | Y | Array elements are IP address strings | Element type is [IPAddr](../00-common.md#7-ipaddr) |
| Config{v}[].KeepAliveParam | Object | The specific keepalive message strategy | Y | - | - |
| Config{v}[].KeepaliveParam.Disable | Boolean | Disable sending keepalive message or not | N | Default value is `false` | - |
| Config{v}[].KeepaliveParam.KeepIdle | Integer | Period to send heartbeat message since there is no data transport in tcp connection | N | Unit: second | Non-negative integer |
| Config{v}[].KeepaliveParam.KeepIntvl | Integer | Period to send heartbeat message again when last message is not applied | N | Unit: second | Positive integer |
| Config{v}[].KeepaliveParam.KeepCnt | Integer | Counter to resend heartbeat message when last message is not applied | N | - | Non-negative integer |

## Configuration Example

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
