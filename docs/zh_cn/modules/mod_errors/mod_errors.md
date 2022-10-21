# mod_errors

## 模块简介

mod_errors根据自定义的条件，将响应内容替换为/重定向至指定错误页。

## 基础配置

### 配置描述

模块配置文件: conf/mod_errors/mod_errors.conf

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Basic.DataPath            | String<br>规则配置的的文件路径 |
| Log.OpenDebug           | Boolean<br>是否开启 debug 日志<br>默认值False |

### 配置示例

```ini
[Basic]
DataPath = mod_errors/errors_rule.data
```

## 规则配置

### 配置描述

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>各产品线的错误响应规则 |
| Config{k} | String<br>产品线名称 |
| Config{v} | Object<br>产品线下的错误响应规则列表 |
| Config{v}[] | Object<br>错误响应规则详细信息 |
| Config{v}[].Cond | String<br>描述匹配请求或连接的条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].Actions | Object<br>匹配成功后的动作|
| Config{v}[].Actions.Cmd | String<br>匹配成功后执行的指令 |
| Config{v}[].Actions.Params | Object<br>执行指令的相关参数列表 |
| Config{v}[].Actions.Params[] | String<br>参数信息 |

### 模块动作

| 动作     | 含义                 |
| -------- | ---------------------- |
| RETURN   | 响应返回指定错误页     |
| REDIRECT | 响应重定向至指定错误页 |

### 配置示例

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
