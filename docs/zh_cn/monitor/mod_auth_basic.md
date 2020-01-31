# 简介

mod_auth_basic支持HTTP基本认证。

# 监控项

| 监控项                   | 描述                                |
| ----------------------- | ---------------------------------- |
| REQ_AUTH_RULE_HIT       | 命中基本认证规则的请求数               |
| REQ_AUTH_CHALLENGE      | 命中规则、未携带AUTHORIZATION头的请求数 |
| REQ_AUTH_SUCCESS        | 认证成功的请求数                      |
| REQ_AUTH_FAILURE        | 认证失败的请求数                      |