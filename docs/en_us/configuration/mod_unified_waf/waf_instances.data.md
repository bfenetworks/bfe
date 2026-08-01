# mod_unified_waf WAF RS Instance Pool Configuration

## Introduction

`waf_instances.data` is used to configure the WAF RS (Real Server) instance pool, including the detection port and health check port.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | - | Type is [Version](../00-common.md#5-version) |
| Config | Object | Detailed configuration, currently only `WafCluster` | Y | - | Must be a non-empty object |
| Config.WafCluster | Array | WafCluster RS configuration list | Y | - | Non-empty array |
| Config.WafCluster[].IpAddr | String | WAF RS IP | Y | - | Type is [IPAddr](../00-common.md#7-ipaddr) |
| Config.WafCluster[].Port | Integer | WAF RS detection port | Y | - | Type is [Port](../00-common.md#1-port) |
| Config.WafCluster[].HealthCheckPort | Integer | WAF RS health check port | Y | - | Type is [Port](../00-common.md#1-port) |

## Configuration Example

```json
{
    "Version": "2023-01-19 12:00:10",
    "Config": {
        "WafCluster": [
            {"IpAddr": "127.0.0.1", "Port": 8899, "HealthCheckPort": 8899}
        ]
    }
}
```
