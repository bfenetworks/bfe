# mod_unified_waf WAF Access Parameter Configuration

## Introduction

`mod_unified_waf.data` is used to configure WAF client parameters, detection parameters, and health checker parameters for accessing WAF RS.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | - | Type is [Version](../00-common.md#5-version) |
| Config | Object | Detailed parameter configuration | Y | - | Must be a non-empty object |
| Config.WafClient | Object | WAF client parameters | Y | - | Must be a non-empty object |
| Config.WafClient.ConnectTimeout | Integer | Connection timeout to WAF RS | Y | Unit: milliseconds | Non-negative integer |
| Config.WafClient.Concurrency | Integer | Concurrency to WAF RS | Y | - | Positive integer |
| Config.WafClient.MaxWaitCount | Integer | Max waiting request count to WAF RS | Y | - | Non-negative integer |
| Config.WafDetect | Object | WAF detection parameters | Y | - | Must be a non-empty object |
| Config.WafDetect.RetryMax | Integer | Max retry count to WAF RS | Y | - | Non-negative integer |
| Config.WafDetect.ReqTimeout | Integer | Request timeout to WAF RS | Y | Unit: milliseconds | Positive integer |
| Config.HealthChecker | Object | WAF RS health checker parameters | Y | - | Must be a non-empty object |
| Config.HealthChecker.UnavailableFailedThres | Integer | Threshold of consecutive failed health checks | Y | - | Positive integer |
| Config.HealthChecker.HealthCheckInterval | Integer | Health check interval | Y | Unit: milliseconds | Positive integer |

## Configuration Example

```json
{
    "Version": "2025-06-23 12:00:10",
    "Config": {
        "WafClient": {
            "ConnectTimeout": 30,
            "Concurrency": 2000,
            "MaxWaitCount": 400
        },
        "WafDetect": {
            "RetryMax": 2,
            "ReqTimeout": 40
        },
        "HealthChecker": {
            "UnavailableFailedThres": 20,
            "HealthCheckInterval": 1000
        }
    }
}
```
