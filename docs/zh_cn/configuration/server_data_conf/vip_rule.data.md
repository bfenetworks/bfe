# VIP规则配置

## 配置简介

vip_rule.data配置文件记录产品线的VIP列表。

## 配置描述

| 配置项    | 类型      | 参数含义                | 必填 | 补充描述                                                     | 合法性条件                                                   |
| --------- | --------- | ----------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version   | String    | 配置文件版本            | Y    | 参见 [Version](../00-common.md#5-配置文件版本version) 类型定义  | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| Vips      | Object    | 各产品线的VIP列表       | Y    | 键为产品线名称，值为该产品线的VIP列表                        | 非空                                                         |
| Vips[k]   | String    | 产品线名称              | Y    | 作为 Vips 的键                                               | 非空                                                         |
| Vips[v]   | []String  | VIP列表                 | Y    | 该产品线下的所有VIP                                          | 非空                                                         |
| Vips[v][] | String    | VIP                     | Y    | 参见 [IPAddr](../00-common.md#7-ip-地址ipaddr) 类型定义         | 类型为 [IPAddr](../00-common.md#7-ip-地址ipaddr)                |

## 配置示例

```json
{
    "Version": "20190101000000",
    "Vips": {
        "example_product": [
            "111.111.111.111"
        ] 
    }
}
```
