# mod_userid 规则配置

## 配置简介

`userid_rule.data` 是 `mod_userid` 模块的规则配置文件，用于配置各产品线下为新用户添加用户标识 Cookie 的规则。

## 配置描述

| 配置项                       | 类型    | 参数含义             | 必填 | 补充描述                                                   | 合法性条件                                           |
| ---------------------------- | ------- | -------------------- | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version                      | String  | 配置文件版本         | Y    | 通常采用时间戳格式，如 `2019-12-10184356`                  | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                       | Object  | 各产品线的规则配置   | Y    | 以产品线名称为键                                           | -                                                    |
| Config{k}                    | String  | 产品线名称           | Y    | -                                                          | -                                                    |
| Config{v}                    | Array   | 产品线规则列表       | Y    | -                                                          | -                                                    |
| Config{v}[]                  | Object  | 一条规则             | Y    | -                                                          | -                                                    |
| Config{v}[].Cond             | String  | 规则条件             | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config{v}[].Params           | Object  | Cookie 参数          | Y    | -                                                          | -                                                    |
| Config{v}[].Params.Name      | String  | Cookie 的 Name 属性  | Y    | -                                                          | -                                                    |
| Config{v}[].Params.Domain    | String  | Cookie 的 Domain 属性 | N    | -                                                          | -                                                    |
| Config{v}[].Params.Path      | String  | Cookie 的 Path 属性  | Y    | -                                                          | -                                                    |
| Config{v}[].Params.MaxAge    | Integer | Cookie 的 MaxAge 属性 | N    | 单位为秒                                                   | -                                                    |
| Config{v}[].Generator        | String  | 用户标识生成器       | N    | 例如 `default`                                             | -                                                    |

## 配置示例

```json
{
    "Version": "2019-12-10184356",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_prefix_in(\"/abc\", true)",
                "Params": {
                     "Name": "bfe_userid_abc",
                     "Domain": "",
                     "Path": "/abc",
                     "MaxAge": 3153600
                 },
                 "Generator": "default"
            },
            {
                "Cond": "default_t()",
                "Params": {
                     "Name": "bfe_userid",
                     "Domain": "",
                     "Path": "/",
                     "MaxAge": 3153600
                 }
            }
        ]
    }
}
```
