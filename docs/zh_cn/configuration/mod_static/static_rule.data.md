# mod_static 规则配置

## 配置简介

`static_rule.data` 是 `mod_static` 模块的规则配置文件。

## 配置描述

| 配置项                       | 类型   | 参数含义                 | 必填 | 补充描述                                                     | 合法性条件                                           |
| ---------------------------- | ------ | ------------------------ | ---- | ------------------------------------------------------------ | ---------------------------------------------------- |
| Version                      | String | 配置文件版本             | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                       | Object | 各产品线的规则列表       | Y    | 以产品线名称为键                                             | -                                                    |
| Config[k]                    | String | 产品线名称               | Y    | -                                                            | -                                                    |
| Config[v]                    | Array  | 产品线的规则列表         | Y    | -                                                            | -                                                    |
| Config[v][]                  | Object | 产品线的规则             | Y    | -                                                            | -                                                    |
| Config[v][].Cond             | String | 规则的匹配条件           | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config[v][].Action           | Object | 规则的执行动作           | Y    | -                                                            | -                                                    |
| Config[v][].Action.Cmd       | String | 动作名称                 | Y    | 合法值包括 `BROWSE`（访问指定目录下的静态文件）            | 取值范围为 `BROWSE`                                  |
| Config[v][].Action.Params    | Array  | 动作参数                 | Y    | -                                                            | -                                                    |
| Config[v][].Action.Params[0] | String | 第一个参数为根目录位置   | Y    | -                                                            | 类型为 [DirPath](../00-common.md#4-目录路径dirpath)     |
| Config[v][].Action.Params[1] | String | 第二个参数为默认静态文件名 | Y    | -                                                            | -                                                    |

## 配置示例

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [
                        "./",
                        "index.html"
                    ]
                }
            }
        ]
    },
    "Version": "20190101000000"
}
```
