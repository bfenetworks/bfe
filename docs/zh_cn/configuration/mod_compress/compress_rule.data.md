# mod_compress 规则配置

## 配置简介

`compress_rule.data` 是 `mod_compress` 模块的规则配置文件。

## 配置描述

| 配置项                       | 类型    | 参数含义             | 必填 | 补充描述                                                   | 合法性条件                                           |
| ---------------------------- | ------- | -------------------- | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version                      | String  | 配置文件版本         | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                       | Object  | 各产品线的压缩规则   | Y    | 以产品线名称为键                                           | -                                                    |
| Config{k}                    | String  | 产品线名称           | Y    | -                                                          | -                                                    |
| Config{v}                    | Array   | 产品线的压缩规则列表 | Y    | -                                                          | -                                                    |
| Config{v}[]                  | Object  | 压缩规则             | Y    | -                                                          | -                                                    |
| Config{v}[].Cond             | String  | 规则条件             | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config{v}[].Action           | Object  | 匹配成功后的动作     | Y    | -                                                          | -                                                    |
| Config{v}[].Action.Cmd       | String  | 动作名称             | Y    | 合法值详见模块动作说明                                     | -                                                    |
| Config{v}[].Action.Quality   | Integer | 压缩级别             | N    | 依具体压缩算法而定                                         | -                                                    |
| Config{v}[].Action.FlushSize | Integer | 压缩过程缓存大小     | N    | 单位为字节                                                 | 正整数                                               |

## 模块动作

| 动作   | 含义         |
| ------ | ------------ |
| GZIP   | gzip 压缩    |
| BROTLI | brotli 压缩  |

## 配置示例

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "GZIP",
                    "Quality": 9,
                    "FlushSize": 512
                }
            }
        ]
    },
    "Version": "20190101000000"
}
```
