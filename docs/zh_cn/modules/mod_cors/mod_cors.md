# mod_cors

## 模块简介

mod_cors支持跨域资源共享

## 基础配置

### 配置描述

模块配置文件: conf/mod_cors/mod_cors.conf

| 配置项         | 描述                               |
| -------------- | ---------------------------------- |
| Basic.DataPath | String<br>规则配置文件路径         |
| Log.OpenDebug  | String<br>是否启用模块调试日志开关 |

### 配置示例

```ini
[Basic]
DataPath = mod_cors/cors_rule.data

[Log]
OpenDebug = false
```

## 规则配置

### 配置描述

规则配置文件: conf/mod_cors/cors_rule.data

| 配置项                     | 描述                                         |
| -------------------------- | -------------------------------------------- |
| Version                    | String<br>配置文件版本                       |
| Config                     | Object<br>各产品线的规则列表                 |
| Config[k]                  | String<br>产品线名称                         |
| Config[v]                  | Object<br>产品线的规则列表                   |
| Config[v][]                | Object<br>产品线的规则                       |
| Config[v][].Cond           | String<br>规则的匹配条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config[v][].AccessControlAllowOrigins    | List<br>告诉浏览器允许哪些网站可以访问跨域资源。<br>"%origin": 表示允许任意域名，且响应Header中Access-Control-Allow-Origin值为请求Header中"Origin"<br>"\*"：表示对于不具备凭证（credentials）的请求，允许所有域名用于资源访问权限"|
| Config[v][].AccessControlAllowCredentials| Boolean<br>是否允许浏览器将对请求的响应暴露给页面           |
| Config[v][].AccessControlExposeHeaders   | Boolean<br>允许客户端访问的响应头列表       |
| Config[v][].AccessControlAllowMethods    | List<br>用于预检请求，表示允许实际请求中客户端使用的方法列表 |
| Config[v][].AccessControlAllowHeaders    | List<br>用于预检请求，表示允许实际请求中客户端使用哪些请求头 |
| Config[v][].AccessControlMaxAge          | Int<br>用于预检请求，表示预检请求返回的结果可以被缓存的时间。-1：表示禁用缓存|

### 配置示例

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
