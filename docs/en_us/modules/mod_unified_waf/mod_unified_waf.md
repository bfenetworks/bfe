# mod_unified_waf

## Introduction

BFE supports integrating unified third-party WAF into the HTTP request processing flow.

## Module Configuration

### Description

Module configuration file: conf/mod_unified_waf/mod_unified_waf.conf

| Config Item | Description |
| ----------- | ----------- |
| Basic.WafProductName | String<br>Name of the third-party WAF product. Candidates: None, BFEMockWaf. Default is None |
| Basic.ConnPoolSize | Integer<br>Connection pool size to WAF server |
| ConfigPath.ModWafDataPath | String<br>Path of WAF access parameter configuration |
| ConfigPath.ProductParamPath | String<br>Path of WAF product configuration |
| ConfigPath.WafInstancesPath | String<br>Path of WAF RS instance pool configuration |
| Log.OpenDebug | Boolean<br>Whether to enable debug logs<br>Default False |

### Example

```ini
[Basic]
#candidates: None, BFEMockWaf
WafProductName = None
ConnPoolSize = 8

[ConfigPath]
ModWafDataPath = "../conf/mod_unified_waf/mod_unified_waf.data"
ProductParamPath = "../conf/mod_unified_waf/product_param.data"
WafInstancesPath = "../conf/mod_unified_waf/waf_instances.data"

[Log]
OpenDebug = false
```

## WAF Access Parameter Configuration

Configuration file: conf/mod_unified_waf/mod_unified_waf.data

### Description

| Config Item | Description |
| ----------- | ----------- |
| Version | String<br>Version of config file |
| Config | Object<br>Detailed parameter configuration |
| Config.WafClient | Object<br>WAF client parameters |
| Config.WafClient.ConnectTimeout | int<br>Connection timeout to WAF RS |
| Config.WafClient.Concurrency | int<br>Concurrency to WAF RS |
| Config.WafClient.MaxWaitCount | int<br>Max waiting request count to WAF RS |
| Config.WafDetect | Object<br>WAF detection parameters |
| Config.WafDetect.RetryMax | int<br>Max retry count to WAF RS |
| Config.WafDetect.ReqTimeout | int<br>Request timeout to WAF RS |
| Config.HealthChecker | Object<br>WAF RS health checker parameters |
| Config.HealthChecker.UnavailableFailedThres | int<br>Threshold of consecutive failed health checks |
| Config.HealthChecker.HealthCheckInterval | int<br>Health check interval (ms) |

### Example

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

## WAF Product Configuration

Configuration file: conf/mod_unified_waf/product_param.data

### Description

| Config Item | Description |
| ----------- | ----------- |
| Version | String<br>Version of config file |
| Config | Object<br>Product configuration |
| Config{k} | String<br>Product name |
| Config{v} | Object<br>Configuration of the product |
| Config{v}.SendBody | Boolean<br>Whether to send body during WAF detection |
| Config{v}.SendBodySize | Integer<br>Max body size to send during WAF detection (byte) |

### Example

```json
{
    "Version": "2023-01-19 12:00:10",
    "Config": {
        "example_product": {
            "SendBody": true,
            "SendBodySize": 1024
        }
    }
}
```

## WAF RS Instance Pool Configuration

Configuration file: conf/mod_unified_waf/waf_instances.data

### Description

| Config Item | Description |
| ----------- | ----------- |
| Version | String<br>Version of config file |
| Config | Object<br>Detailed configuration, currently only WafCluster |
| Config.WafCluster | Object<br>WafCluster RS configuration |
| Config.WafCluster[].IpAddr | String<br>WAF RS IP |
| Config.WafCluster[].Port | Integer<br>WAF RS detection port |
| Config.WafCluster[].HealthCheckPort | Integer<br>WAF RS health check port |

### Example

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

## Metrics

| Metric | Description |
| ------ | ----------- |
| REQ_NO_CHECK | Count of requests not checked by WAF |
| REQ_FORBIDDEN | Count of requests blocked by WAF |
| REQ_OK | Count of requests judged normal by WAF |
| REQ_TIMEOUT | Count of WAF detection timeouts |
| REQ_OTHER | Count of other status requests |
| NET_ERR | Count of WAF detection network errors |

The module also records the following delays via delay counter (key prefix):

| Delay Metric | Description |
| ------------ | ----------- |
| waf_client_delay | WAF request-response delay |
| waf_client_delay_peek_body | Body read delay |
| waf_client_delay_call_competition | Concurrency competition delay |
