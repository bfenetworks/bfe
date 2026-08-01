# mod_tag 规则配置

## 配置简介

`tag_rule.data` 是 `mod_tag` 模块的规则配置文件。

## 配置描述

| 配置项                      | 类型    | 参数含义                                         | 必填 | 补充描述                                                   | 合法性条件                                           |
| --------------------------- | ------- | ------------------------------------------------ | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version                     | String  | 配置文件版本                                     | Y    | 通常采用时间戳格式，如 `20200218210000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                      | Object  | 各产品线的规则列表                               | Y    | 以产品线名称为键                                           | -                                                    |
| Config[k]                   | String  | 产品线名称                                       | Y    | -                                                          | -                                                    |
| Config[v]                   | Array   | 产品线的规则列表                                 | Y    | -                                                          | -                                                    |
| Config[v][]                 | Object  | 产品线的规则                                     | Y    | -                                                          | -                                                    |
| Config[v][].Cond            | String  | 规则的匹配条件                                   | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config[v][].Param.TagName   | String  | 标签名称                                         | Y    | -                                                          | -                                                    |
| Config[v][].Param.TagValue  | String  | 标签取值                                         | Y    | -                                                          | -                                                    |
| Config[v][].Last            | Boolean | 设置为 true 时，命中当前规则后停止处理后续规则   | N    | 默认值为 `false`                                           | -                                                    |

## 配置示例

```json
{
  "Version": "20200218210000",
  "Config": {
    "example_product": [
      {
        "Cond": "req_host_in(\"example.org\")",
        "Param": {
          "TagName": "tag",
          "TagValue": "bfe"
        },
        "Last": false
      }
    ]
  }
}
```
