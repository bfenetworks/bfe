# mod_secure_link

## 模块简介

mod_secure_link 校验请求链接是否授权，保护链接不被未授权访问，同时还限制链接的有效期。

## 基础配置

模块基础配置文件说明详见 [mod_secure_link.conf](../../configuration/mod_secure_link/mod_secure_link.conf.md)。

## 规则配置

模块规则配置文件说明详见 [secure_link_rule.data](../../configuration/mod_secure_link/secure_link_rule.data.md)。

## 监控项

| 监控项                     | 描述                            |
| -------------------------- | ------------------------------- |
| REQ_TOTAL                  | 请求总数                        |
| REQ_ACCEPT                 | 校验通过的请求数                |
| REQ_WITHOUT_EXPIRES_KEY    | 缺少 expires 参数的请求数       |
| REQ_INVALID_EXPIRES_VALUE  | expires 参数值非法的请求数      |
| REQ_WITHOUT_CHECKSUM_KEY   | 缺少 checksum 参数的请求数      |
| REQ_INVALID_CHECKSUM       | checksum 校验失败的请求数       |
| REQ_EXPIRED                | 链接已过期的请求数              |
