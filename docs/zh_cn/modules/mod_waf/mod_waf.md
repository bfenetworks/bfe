# mod_waf

## 模块简介

mod_waf 基于自定义规则，对请求执行 WAF（Web 应用防火墙）检测。支持 block 规则（命中后直接拦截）和 check 规则（命中后记录日志）。

## 基础配置

模块基础配置文件说明详见 [mod_waf.conf](../../configuration/mod_waf/mod_waf.conf.md)。

## 规则配置

模块规则配置文件说明详见 [waf_rule.data](../../configuration/mod_waf/waf_rule.data.md)。

## 监控项

| 监控项           | 描述                     |
| ---------------- | ------------------------ |
| CHECKED_REQ      | WAF 检测的请求数         |
| HIT_BLOCKED_REQ  | 命中拦截规则的请求数     |
| HIT_CHECKED_RULE | 命中检测规则的请求数     |
| BLOCKED_RULE_ERROR | 拦截规则执行错误次数     |
| CHECKED_RULE_ERROR | 检测规则执行错误次数     |
