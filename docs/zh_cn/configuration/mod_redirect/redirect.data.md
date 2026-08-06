# mod_redirect 规则配置

## 配置简介

`redirect.data` 是 `mod_redirect` 模块的规则配置文件。

## 配置描述

| 配置项                       | 类型    | 参数含义             | 必填 | 补充描述                                                   | 合法性条件                                           |
| ---------------------------- | ------- | -------------------- | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version                      | String  | 配置文件版本         | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                       | Object  | 各产品线的重定向规则 | Y    | 以产品线名称为键                                           | -                                                    |
| Config{k}                    | String  | 产品线名称           | Y    | -                                                          | -                                                    |
| Config{v}                    | Array   | 产品线重定向规则表   | Y    | -                                                          | -                                                    |
| Config{v}[]                  | Object  | 产品线重定向规则     | Y    | -                                                          | -                                                    |
| Config{v}[].Cond             | String  | 规则条件             | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config{v}[].Actions          | Array   | 规则动作列表         | Y    | -                                                          | -                                                    |
| Config{v}[].Actions[].Cmd    | String  | 规则动作名称         | Y    | 合法值详见模块动作说明                                     | -                                                    |
| Config{v}[].Actions[].Params | Object  | 规则动作参数         | N    | 依具体动作而定                                             | -                                                    |
| Config{v}[].Status           | Integer | HTTP 状态码          | N    | -                                                          | 合法 HTTP 重定向状态码                               |

## 模块动作

| 动作           | 描述                                              |
| -------------- | ------------------------------------------------- |
| URL_SET        | 设置重定向URL为指定值                             |
| URL_FROM_QUERY | 设置重定向URL为指定请求Query值                    |
| URL_PREFIX_ADD | 设置重定向URL为原始URL增加指定前缀                |
| SCHEME_SET     | 设置重定向URL为原始URL并修改协议(支持HTTP和HTTPS) |

## 配置示例

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_prefix_in(\"/redirect\", false)",
                "Actions": [
                    {
                        "Cmd": "URL_SET",
                        "Params": ["https://example.org"]
                    }
                ],
                "Status": 301
            }
        ]
    }
}
```
