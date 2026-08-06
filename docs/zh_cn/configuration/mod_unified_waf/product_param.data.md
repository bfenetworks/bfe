# mod_unified_waf WAF 访问产品线配置

## 配置简介

`product_param.data` 用于配置各产品线在 WAF 检测时的行为参数。

## 配置描述

| 配置项             | 类型    | 参数含义                       | 必填 | 补充描述 | 合法性条件 |
| ------------------ | ------- | ------------------------------ | ---- | -------- | ---------- |
| Version            | String  | 配置文件版本                   | Y    | -        | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config             | Object  | 各产品线的参数配置             | Y    | -        | 不可为空对象 |
| Config{k}          | String  | 产品线名称                     | Y    | 作为 Config 的键 | 非空字符串 |
| Config{v}          | Object  | 该产品线的配置                 | Y    | -        | 不可为空对象 |
| Config{v}.SendBody | Boolean | WAF 检测时，是否发送 body      | Y    | -        | - |
| Config{v}.SendBodySize | Integer | WAF 检测时，发送 body 的最大大小 | Y    | 单位：字节 | 非负整数 |

## 配置示例

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
