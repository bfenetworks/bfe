# mod_cors 规则配置

## 配置简介

`cors_rule.data` 是 `mod_cors` 模块的规则配置文件。

## 配置描述

| 配置项                                    | 类型    | 参数含义                                                     | 必填 | 补充描述                                                     | 合法性条件         |
| ----------------------------------------- | ------- | ------------------------------------------------------------ | ---- | ------------------------------------------------------------ | ------------------ |
| Version                                   | String  | 配置文件版本                                                 | Y    | 通常采用时间戳格式，如 `20190101000000`                      | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                                    | Object  | 各产品线的 CORS 规则                                         | Y    | 以产品线名称为键                                             | -                  |
| Config{k}                                 | String  | 产品线名称                                                   | Y    | -                                                            | -                  |
| Config{v}                                 | Array   | 产品线的 CORS 规则列表                                       | Y    | -                                                            | -                  |
| Config{v}[]                               | Object  | CORS 规则                                                    | Y    | -                                                            | -                  |
| Config{v}[].Cond                          | String  | 规则条件                                                     | Y    | 语法详见 [Condition](../../condition/condition_grammar.md)   | -                  |
| Config{v}[].AccessControlAllowOrigins     | Array   | 允许访问跨域资源的 Origin 列表                               | N    | `"%origin"` 表示允许任意域名并使用请求 `Origin` 值；`"*"` 表示对不带凭证的请求允许所有域名 | -                  |
| Config{v}[].AccessControlAllowCredentials | Boolean | 是否允许浏览器将对请求的响应暴露给页面                       | N    | -                                                            | -                  |
| Config{v}[].AccessControlExposeHeaders    | Array   | 允许客户端访问的响应头列表                                   | N    | -                                                            | -                  |
| Config{v}[].AccessControlAllowMethods     | Array   | 用于预检请求，表示允许实际请求中客户端使用的方法列表         | N    | -                                                            | -                  |
| Config{v}[].AccessControlAllowHeaders     | Array   | 用于预检请求，表示允许实际请求中客户端使用的请求头列表       | N    | -                                                            | -                  |
| Config{v}[].AccessControlMaxAge           | Integer | 用于预检请求，表示预检请求返回结果可被缓存的时间（秒）       | N    | `-1` 表示禁用缓存                                            | 大于等于 `-1`      |

## 配置示例

```json
{
    "Version": "cors_rule.data.version",
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"example.org\")",
                "AccessControlAllowOrigins": ["%origin"],
                "AccessControlAllowCredentials": true,
                "AccessControlExposeHeaders": ["X-Custom-Header"],
                "AccessControlAllowMethods": ["HEAD","GET","POST","PUT","DELETE","OPTIONS","PATCH"],
                "AccessControlAllowHeaders": ["X-Custom-Header"],
                "AccessControlMaxAge": -1
            }
        ]
    }
}
```
