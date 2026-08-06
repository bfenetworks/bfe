# mod_trace 规则配置

## 配置简介

`trace_rule.data` 是 `mod_trace` 模块的规则配置文件，用于配置各产品线下是否开启分布式跟踪。

## 配置描述

| 配置项             | 类型    | 参数含义           | 必填 | 补充描述                                                   | 合法性条件                                           |
| ------------------ | ------- | ------------------ | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version            | String  | 配置文件版本       | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config             | Object  | 各产品线的规则列表 | Y    | 以产品线名称为键                                           | -                                                    |
| Config{k}          | String  | 产品线名称         | Y    | -                                                          | -                                                    |
| Config{v}          | Array   | 产品线的规则列表   | Y    | -                                                          | -                                                    |
| Config{v}[]        | Object  | 一条规则           | Y    | -                                                          | -                                                    |
| Config{v}[].Cond   | String  | 规则的匹配条件     | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config{v}[].Enable | Boolean | 是否开启 trace     | Y    | -                                                          | -                                                    |

## 配置示例

```json
{
  "Version": "20200218210000",
  "Config": {
    "example_product": [
       {
         "Cond": "req_host_in(\"example.org\")",
         "Enable": true
       }
    ]
  }
}
```
