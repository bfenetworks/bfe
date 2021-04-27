# VIP规则配置

## 配置简介

vip_rule.data配置文件记录产品线的VIP列表。

## 配置描述

| 配置项    | 描述                                 |
| --------- | ------------------------------------ |
| Version   | String<br>配置文件版本               |
| Vips      | Object<br>各产品线的VIP列表          |
| Vips[k]   | String<br>产品线名称                 |
| Vips[v]   | String<br>VIP列表                    |
| Vips[v][] | String<br>VIP                        |

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
