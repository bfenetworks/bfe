# mod_block 封禁规则配置

## 配置简介

`block_rules.data` 是 `mod_block` 模块的封禁规则配置文件，用于配置各产品线下的封禁规则。

## 配置描述

| 配置项                       | 类型    | 参数含义             | 必填 | 补充描述                                                   | 合法性条件                                           |
| ---------------------------- | ------- | -------------------- | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version                      | String  | 配置文件版本         | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                       | Object  | 各产品线的封禁规则   | Y    | 以产品线名称为键                                           | -                                                    |
| Config{k}                    | String  | 产品线名称           | Y    | -                                                          | -                                                    |
| Config{v}                    | Array   | 产品线下的封禁规则列表 | Y    | -                                                          | -                                                    |
| Config{v}[]                  | Object  | 一条封禁规则         | Y    | -                                                          | -                                                    |
| Config{v}[].Cond             | String  | 规则匹配条件         | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config{v}[].Name             | String  | 规则名称             | Y    | -                                                          | -                                                    |
| Config{v}[].Action           | Object  | 匹配成功后的动作     | Y    | -                                                          | -                                                    |
| Config{v}[].Action.Cmd       | String  | 匹配成功后执行的指令 | Y    | 合法值详见模块动作说明                                     | 须为 `CLOSE`、`ALLOW` 之一                           |
| Config{v}[].Action.Params    | Array   | 执行指令的参数列表   | N    | 参数依具体指令而定；元素类型为 String                      | -                                                    |

## 模块动作

| 动作  | 含义     |
| ----- | -------- |
| CLOSE | 关闭连接 |
| ALLOW | 允许请求 |

## 配置示例

```json
{
    "Version": "20190101000000",
    "Config": {
        "global": [
            {
                "action": {
                    "cmd": "ALLOW",
                    "params": []
                },
                "cond": "req_host_in(\"n.example.org\") && req_path_prefix_in(\"/index/\", false) && req_query_key_in(\"space\")",
                "name": "example whiterule"
            }
        ],
        "example_product": [
            {
                "action": {
                    "cmd": "CLOSE",
                    "params": []
                },
                "name": "example rule",
                "cond": "req_path_in(\"/limit\", false)"
            }
        ]
    }
}
```
