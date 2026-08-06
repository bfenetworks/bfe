# mod_trust_clientip 信任 IP 字典配置

## 配置简介

`trust_client_ip.data` 是 `mod_trust_clientip` 模块的信任 IP 字典配置文件，用于配置所有信任的 IP 段列表。

## 配置描述

| 配置项            | 类型   | 参数含义         | 必填 | 补充描述         | 合法性条件                                           |
| ----------------- | ------ | ---------------- | ---- | ---------------- | ---------------------------------------------------- |
| Version           | String | 配置文件版本     | Y    | 通常采用时间戳格式，如 `20190101000000` | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config            | Object | 所有信任的 IP 列表 | Y    | 以地址标签为键   | -                                                    |
| Config{k}         | String | 地址标签         | Y    | -                | -                                                    |
| Config{v}         | Array  | 信任的 IP 段列表 | Y    | -                | -                                                    |
| Config{v}[]       | Object | 一个 IP 段       | Y    | -                | -                                                    |
| Config{v}[].Begin | String | IP 段起始地址    | Y    | -                | 类型为 [IPAddr](../00-common.md#7-ip-地址ipaddr)；须小于等于结束地址 |
| Config{v}[].End   | String | IP 段结束地址    | Y    | -                | 类型为 [IPAddr](../00-common.md#7-ip-地址ipaddr)；须大于等于起始地址 |

## 配置示例

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
