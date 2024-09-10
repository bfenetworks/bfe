# mod_redirect

## 模块简介

mod_rediect根据自定义的条件，对请求进行重定向。

## 基础配置

### 配置描述

模块配置文件: conf/mod_redirect/mod_redirect.conf

| 配置项         | 描述                               |
| -------------- | ---------------------------------- |
| Basic.DataPath | String<br>规则配置文件路径         |

### 配置示例

```ini
[Basic]
DataPath = mod_redirect/redirect.data
```

## 规则配置

### 配置描述

规则配置文件: conf/mod_redirect/redirect.data

| 配置项                     | 描述                           |
| -------------------------- | ------------------------------ |
| Version                    | String<br>配置文件版本         |
| Config                     | Object<br>各产品线的重定向规则 |
| Config{k}                  | String<br>产品线名称           |
| Config{v}                  | String<br>产品线重定向规则表   |
| Config{v}[]                | String<br>产品线重定向规则     |
| Config{v}[].Cond           | String<br>规则条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].Actions        | Object<br>规则动作             |
| Config{v}[].Actions.Cmd    | String<br>规则动作名称         |
| Config{v}[].Actions.Params | Object<br>规则动作参数         |
| Config{v}[].Status         | Integer<br>HTTP状态码          |

### 模块动作

| 动作           | 描述                                              |
| -------------- | ------------------------------------------------- |
| URL_SET        | 设置重定向URL为指定值                             |
| URL_FROM_QUERY | 设置重定向URL为指定请求Query值                    |
| URL_PREFIX_ADD | 设置重定向URL为原始URL增加指定前缀                |
| SCHEME_SET     | 设置重定向URL为原始URL并修改协议(支持HTTP和HTTPS) |

### 配置示例

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
