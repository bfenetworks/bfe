# mod_auth_request

## 模块简介

mod_auth_request支持请求发送至指定的服务进行认证。

## 基础配置

模块基础配置文件说明详见 [mod_auth_request.conf](../../configuration/mod_auth_request/mod_auth_request.conf.md)。

## 规则配置

模块规则配置文件说明详见 [auth_request_rule.data](../../configuration/mod_auth_request/auth_request_rule.data.md)。

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
