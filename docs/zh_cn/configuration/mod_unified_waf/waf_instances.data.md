# mod_unified_waf WAF RS 实例池配置

## 配置简介

`waf_instances.data` 用于配置 WAF RS（Real Server）实例池，包括检测端口和健康检查端口。

## 配置描述

| 配置项                       | 类型    | 参数含义             | 必填 | 补充描述 | 合法性条件 |
| ---------------------------- | ------- | -------------------- | ---- | -------- | ---------- |
| Version                      | String  | 配置文件版本         | Y    | -        | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                       | Object  | 实例池配置           | Y    | -        | 不可为空对象 |
| Config.WafCluster            | Array   | WafCluster RS 配置列表 | Y    | -        | 非空数组 |
| Config.WafCluster[].IpAddr   | String  | WAF RS IP 地址       | Y    | -        | 类型为 [IPAddr](../00-common.md#7-ip-地址ipaddr) |
| Config.WafCluster[].Port     | Integer | WAF RS 攻击检测端口  | Y    | -        | 类型为 [Port](../00-common.md#1-网络端口port) |
| Config.WafCluster[].HealthCheckPort | Integer | WAF RS 健康检测端口 | Y    | -        | 类型为 [Port](../00-common.md#1-网络端口port) |

## 配置示例

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
