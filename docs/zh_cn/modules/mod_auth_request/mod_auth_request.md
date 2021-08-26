# mod_auth_request

## 模块简介

mod_auth_request支持请求发送至产品线指定的服务进行认证。

## 基础配置
### 配置描述
模块配置文件: conf/mod_auth_request/mod_auth_request.conf

| 配置项            | 描述                                            |
| ----------------- | ----------------------------------------------- |
| Basic.DataPath    | String<br>规则配置的文件路径                    |
| Basic.AuthAddress | String<br>认证服务的地址                        |
| Basic.AuthTimeout | Number<br>认证超时时间<br>单位ms                |
| Log.OpenDebug     | Boolean<br/>是否开启 debug 日志<br/>默认值False |

### 配置示例

```ini
[Basic]
DataPath = mod_auth_request/auth_request_rule.data
AuthAddress = http://127.0.0.1
AuthTimeout = 100

[Log]
OpenDebug = false
```

## 规则配置
### 配置描述
| 配置项             | 描述                                                         |
| ------------------ | ------------------------------------------------------------ |
| Version            | String<br>配置文件版本                                       |
| Config             | Object<br>所有产品线的请求认证规则配置                       |
| Config{k}          | String<br>产品线名称                                         |
| Config{v}          | Object<br> 产品线下请求认证规则列表                          |
| Config{v}[]        | Object<br> 请求认证规则                                      |
| Config{v}[].Cond   | String<br>匹配条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].Enable | Boolean<br>是否启用规则                                      |

### 配置示例

```josn
{
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_in(\"/auth_request\", false)",
                "Enable": true
            }
        ]
    },
    Version": "20190101000000"
}
```

### 配置说明

对于example_product产品线，配置了一条规则，规则处于启用状态，请求路径中带有/auth_request的请求，比如www.example.com/auth_request，BFE就会构造请求发送至http://127.0.0.1并接收响应。

### BFE构造请求的方式说明

* Method: BFE构造的请求Method为Get。
* Header: 请求Header为原请求Header去掉Key为"Content-Length"、"Connection"、"Keep-Alive"、"Proxy-Authenticate"、"Proxy-Authorization"、"Te"、"Trailers"、"Transfer-Encoding"、"Upgrade"的Header，并且如果原请求不携带"X-Forwarded-Method"、"X-Forwarded-Uri"，BFE会添加Key为"X-Forwarded-Method"且Value为原请求Method、Key为"X-Forwarded-Uri"且Value为原请求URI的Header。
* Body: BFE构造的请求Body为空。

### 模块动作

| 动作 | 条件                  |
| ---- | --------------------- |
| 封禁 | 响应状态码为401或403  |
| 放行 | 响应状态码为200或其他 |

## 监控项

| 监控项                    | 描述                     |
| ------------------------- | ------------------------ |
| AUTH_REQUEST_CHECKED      | 命中基本认证规则的请求数 |
| AUTH_REQUEST_PASS         | 认证成功并放行的请求数   |
| AUTH_REQUEST_FORBIDDEN    | 被禁止的请求数           |
| AUTH_REQUEST_UNAUTHORIZED | 未通过认证的请求数       |
| AUTH_REQUEST_FAIL         | 认证失败的请求数         |
| AUTH_REQUEST_UNCERTAIN    | 认证状态不确定的请求数   |