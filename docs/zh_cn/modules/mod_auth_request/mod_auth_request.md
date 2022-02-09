# mod_auth_request

## 模块简介

mod_auth_request支持请求发送至指定的服务进行认证。

## 基础配置

### 配置描述

模块配置文件: conf/mod_auth_request/mod_auth_request.conf

| 配置项            | 描述                                            |
| ----------------- | ----------------------------------------------- |
| Basic.DataPath    | String<br>规则配置的文件路径                    |
| Basic.AuthAddress | String<br>认证服务的地址                        |
| Basic.AuthTimeout | Number<br>认证超时时间<br>单位ms                |
| Log.OpenDebug     | Boolean<br/>是否开启调试日志<br/>默认值False |

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
| Config{v}          | Object<br> 产品线的请求认证规则表                          |
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

对于example_product产品线配置了一条规则，针对请求路径为/auth_request的请求（例如www.example.com/auth_request），BFE将构造请求发送至http://127.0.0.1进行认证。

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

## BFE构造请求的说明

* Method: BFE构造的请求Method为GET
* Header: BFE构造的请求Header为原请求Header，同时进行如下修改：
  * 删除如下头部：Content-Length/Connection/Keep-Alive/Proxy-Authenticate/Proxy-Authorization/Te/Trailers/Transfer-Encoding/Upgrade
  * 增加如下头部：X-Forwarded-Method(代表原请求Method）、X-Forwarded-Uri（代表原请求URI）
* Body: BFE构造的请求Body为空
