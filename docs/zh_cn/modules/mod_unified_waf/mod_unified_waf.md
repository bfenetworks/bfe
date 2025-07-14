# mod_unified_waf

## 模块简介

BFE 支持在 http request 的处理流程中引入统一的第三方WAF支持。

## 基础配置

### 配置描述

模块配置文件: conf/mod_unified_waf/mod_unified_waf.conf

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Basic.WafProductName            | String<br> 第三方WAF产品的名字，默认提供None、BFEMockWaf两个候选。默认值为None |
| Basic.ConnPoolSize            | String<br> 与WAF server 的连接池大小 |
| ConfigPath.ModWafDataPath      | String<br> WAF访问的具体参数配置 |
| ConfigPath.ProductParamPath      | String<br> WAF访问的产品线配置 |
| ConfigPath.WafInstancesPath      | String<br> WAF RS实例池的配置 |
| Log.OpenDebug           | Boolean<br>是否开启 debug 日志<br>默认值False |

### 配置示例

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

## WAF访问具体参数配置
配置文件: conf/mod_unified_waf/mod_unified_waf.data
### 配置描述

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>具体参数配置 |
| Config.WafClient | Object<br> WAF Client参数配置 |
| Config.WafClient.ConnectTimeout | int<br>连接 WAF RS的超时时间|
| Config.WafClient.Concurrency | int<br>访问 WAF RS的并发度|
| Config.WafClient.MaxWaitCount | int<br>访问 WAF RS的等待请求数|
| Config.WafDetect | Object<br> WAF 检测参数配置 |
| Config.WafDetect.RetryMax | int<br> 访问 WAF RS的重试次数 |
| Config.WafDetect.ReqTimeout | int<br> 访问 WAF RS的超时时间|
| Config.HealthChecker | Object<br> WAF RS 健康检查参数配置 |
| Config.HealthChecker.UnavailableFailedThres | int<br> WAF RS健康检测时，RS不可访问的连续失败次数阈值 |
| Config.HealthChecker.HealthCheckInterval | int<br> WAF RS健康检测的间隔（ms） |


### 配置示例

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

## WAF访问产品线配置
配置文件: conf/mod_unified_waf/product_param.data

### 配置描述

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>具体参数配置 |
| Config{k} | Object<br> 具体产品线的名字 |
| Config{v} | Object<br> 具体产品线的配置 |
| Config{v}.SendBody | Object<br> WAF 检测时，是否发送body |
| Config{v}.SendBodySize | Object<br> WAF 检测时，发送body的最大size(byte) |


### 配置示例

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


## WAF RS实例池配置
配置文件: conf/mod_unified_waf/waf_instances.data

### 配置描述

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br> 具体配置信息，目前只有一个WafCluster|
| Config.WafCluster | Object<br> WafCluster RS 具体配置 |
| Config.WafCluster[].IpAddr | String<br> WAF RS IP |
| Config.WafCluster[].Port | String<br> WAF RS 攻击检测端口 |
| Config.WafCluster[].HealthCheckPort | String<br> WAF RS 健康检测端口 |

### 配置示例

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