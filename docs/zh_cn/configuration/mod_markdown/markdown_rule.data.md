# mod_markdown 规则配置

## 配置简介

`markdown_rule.data` 是 `mod_markdown` 模块的规则配置文件。

## 配置描述

| 配置项        | 类型   | 参数含义                   | 必填 | 补充描述                                                   | 合法性条件                                           |
| ------------- | ------ | -------------------------- | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version       | String | 配置文件版本               | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config        | Object | 各产品线的 Markdown 渲染规则 | Y    | 以产品线名称为键                                           | -                                                    |
| Config{k}     | String | 产品线名称                 | Y    | -                                                          | -                                                    |
| Config{v}     | Object | 产品线下的规则列表         | Y    | -                                                          | -                                                    |
| Config{v}[]   | Object | 规则详细信息               | Y    | -                                                          | -                                                    |
| Config{v}[].Cond | String | 描述匹配请求的条件         | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |

## 配置示例

```json
{
    "Version": "123",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_in(\"/md\", false)"
            }
        ]
    }
}
```
