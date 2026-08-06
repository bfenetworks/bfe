# mod_auth_jwt

## 模块简介

mod_auth_jwt支持JWT([JSON Web Token](https://tools.ietf.org/html/rfc7519))认证

## 基础配置

模块基础配置文件说明详见 [mod_auth_jwt.conf](../../configuration/mod_auth_jwt/mod_auth_jwt.conf.md)。

## 规则配置

模块规则配置文件说明详见 [auth_jwt_rule.data](../../configuration/mod_auth_jwt/auth_jwt_rule.data.md)。

## 监控项

| 监控项                            | 描述                            |
| --------------------------------- | ------------------------------- |
| REQ_AUTH_RULE_HIT                 | 命中鉴权规则的请求数            |
| REQ_AUTH_NO_AUTHORIZATION         | 未携带 Authorization 的请求数   |
| REQ_AUTH_AUTHORIZATION_FORMAT_ERR | Authorization 格式错误的请求数  |
| REQ_AUTH_SUCCESS                  | 鉴权成功的请求数                |
| REQ_AUTH_FAILURE                  | 鉴权失败的请求数                |
