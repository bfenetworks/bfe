# mod_unified_waf WAF 访问具体参数配置

## 配置简介

`mod_unified_waf.data` 用于配置访问 WAF RS 的客户端参数、检测参数以及健康检查参数。

## 配置描述

| 配置项                                      | 类型    | 参数含义                                           | 必填 | 补充描述 | 合法性条件 |
| ------------------------------------------- | ------- | -------------------------------------------------- | ---- | -------- | ---------- |
| Version                                     | String  | 配置文件版本                                       | Y    | -        | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                                      | Object  | 具体参数配置                                       | Y    | -        | 不可为空对象 |
| Config.WafClient                            | Object  | WAF Client 参数配置                                | Y    | -        | 不可为空对象 |
| Config.WafClient.ConnectTimeout             | Integer | 连接 WAF RS 的超时时间                             | Y    | 单位：毫秒 | 非负整数 |
| Config.WafClient.Concurrency                | Integer | 访问 WAF RS 的并发度                               | Y    | -        | 正整数 |
| Config.WafClient.MaxWaitCount               | Integer | 访问 WAF RS 的等待请求数                           | Y    | -        | 非负整数 |
| Config.WafDetect                            | Object  | WAF 检测参数配置                                   | Y    | -        | 不可为空对象 |
| Config.WafDetect.RetryMax                   | Integer | 访问 WAF RS 的重试次数                             | Y    | -        | 非负整数 |
| Config.WafDetect.ReqTimeout                 | Integer | 访问 WAF RS 的超时时间                             | Y    | 单位：毫秒 | 正整数 |
| Config.HealthChecker                        | Object  | WAF RS 健康检查参数配置                            | Y    | -        | 不可为空对象 |
| Config.HealthChecker.UnavailableFailedThres | Integer | WAF RS 健康检测时，RS 不可访问的连续失败次数阈值   | Y    | -        | 正整数 |
| Config.HealthChecker.HealthCheckInterval    | Integer | WAF RS 健康检测的间隔                              | Y    | 单位：毫秒 | 正整数 |

## 配置示例

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
