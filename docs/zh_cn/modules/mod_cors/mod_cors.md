# mod_cors

## 模块简介

mod_cors支持跨域资源共享

## 基础配置

模块基础配置文件说明详见 [mod_cors.conf](../../configuration/mod_cors/mod_cors.conf.md)。

## 规则配置

模块规则配置文件说明详见 [cors_rule.data](../../configuration/mod_cors/cors_rule.data.md)。

## 监控项

| 监控项                  | 描述                       |
| ----------------------- | -------------------------- |
| REQ_CORS_RULE_HIT       | 命中 CORS 规则的请求数     |
| REQ_PRE_FLIGHT_HIT      | 命中预检请求的请求数       |
| REQ_ALLOW_ORIGIN_HIT    | 命中允许 Origin 的请求数   |
| REQ_NOT_ALLOW_ORIGIN_HIT| 未命中允许 Origin 的请求数 |
