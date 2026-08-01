# mod_errors 规则配置

## 配置简介

`errors_rule.data` 是 `mod_errors` 模块的规则配置文件，用于为各产品线配置错误响应替换或重定向规则。

## 配置描述

| 配置项                       | 类型    | 参数含义                     | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ---------------------------- | ------- | ---------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version                      | String  | 配置文件版本                 | Y    | 通常采用时间戳格式                                           | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| Config                       | Object  | 各产品线的错误响应规则       | Y    | key 为产品线名称                                             | -                                                            |
| Config{k}                    | String  | 产品线名称                   | Y    | -                                                            | -                                                            |
| Config{v}                    | Object  | 该产品线下的错误响应规则列表 | Y    | -                                                            | -                                                            |
| Config{v}[]                  | Object  | 错误响应规则详细信息         | Y    | -                                                            | -                                                            |
| Config{v}[].Cond             | String  | 匹配请求或连接的条件         | Y    | 语法详见 [Condition](../../condition/condition_grammar.md)   | 须为合法的 Condition 表达式                                  |
| Config{v}[].Actions          | Array   | 匹配成功后执行的动作列表     | Y    | -                                                            | -                                                            |
| Config{v}[].Actions[].Cmd    | String  | 匹配成功后执行的指令         | Y    | 取值：`RETURN` / `REDIRECT`                                  | 须为 `RETURN` 或 `REDIRECT`                                  |
| Config{v}[].Actions[].Params | Array   | 指令相关参数列表             | Y    | `RETURN` 须 3 个参数；`REDIRECT` 须 1 个参数                 | 参数须符合对应指令要求                                       |
| Config{v}[].Actions[].Params[] | String | 单个参数信息                | Y    | -                                                            | -                                                            |

## 模块动作

| 动作     | 含义                 |
| -------- | -------------------- |
| RETURN   | 响应返回指定错误页   |
| REDIRECT | 响应重定向至指定错误页 |

## 配置示例

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "res_code_in(\"404\")",
                "Actions": [
                    {
                        "Cmd": "RETURN",
                        "Params": [
                            "200", "text/html", "../conf/mod_errors/404.html"
                        ]
                    }
                ]
            },
            {
                "Cond": "res_code_in(\"500\")",
                "Actions": [
                    {
                        "Cmd": "REDIRECT",
                        "Params": [
                            "http://example.org/error.html"
                        ]
                    }
                ]
            }
        ]
    }
}
```
