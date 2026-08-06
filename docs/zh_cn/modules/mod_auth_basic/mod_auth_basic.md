# mod_auth_basic

## 模块简介

mod_auth_basic支持HTTP基本认证。

## 基础配置

模块基础配置文件说明详见 [mod_auth_basic.conf](../../configuration/mod_auth_basic/mod_auth_basic.conf.md)。

## 规则配置

模块规则配置文件说明详见 [auth_basic_rule.data](../../configuration/mod_auth_basic/auth_basic_rule.data.md)。

## 监控项

| 监控项                   | 描述                                |
| ----------------------- | ---------------------------------- |
| REQ_AUTH_RULE_HIT       | 命中基本认证规则的请求数               |
| REQ_AUTH_CHALLENGE      | 命中规则、未携带AUTHORIZATION头的请求数 |
| REQ_AUTH_SUCCESS        | 认证成功的请求数                      |
| REQ_AUTH_FAILURE        | 认证失败的请求数                      |
