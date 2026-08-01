# mod_auth_request 规则配置

## 配置简介

`auth_request_rule.data` 是 `mod_auth_request` 模块的规则配置文件，用于按产品线配置请求认证规则。

## 配置描述

| 配置项             | 类型    | 参数含义                     | 必填 | 补充描述                                                   | 合法性条件                                           |
| ------------------ | ------- | ---------------------------- | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version            | String  | 配置文件版本                 | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config             | Object  | 所有产品线的请求认证规则配置 | Y    | 以产品线名称为键                                           | -                                                    |
| Config{k}          | String  | 产品线名称                   | Y    | -                                                          | -                                                    |
| Config{v}          | Array   | 产品线的请求认证规则表       | Y    | -                                                          | -                                                    |
| Config{v}[]        | Object  | 请求认证规则                 | Y    | -                                                          | -                                                    |
| Config{v}[].Cond   | String  | 匹配条件                     | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config{v}[].Enable | Boolean | 是否启用规则                 | Y    | -                                                          | -                                                    |

## 配置示例

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_in(\"/auth_request\", false)",
                "Enable": true
            }
        ]
    }
}
```
