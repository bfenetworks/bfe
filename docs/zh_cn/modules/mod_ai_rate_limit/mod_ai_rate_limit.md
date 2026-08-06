# mod_ai_rate_limit

## 模块简介

mod_ai_rate_limit 用于对 AI 请求进行限流。支持基于 Redis 的分布式限流，可按产品、apikey 等维度配置 TPM（每分钟 Token 数）、RPM（每分钟请求数）和最大并发数限制。

## 基础配置

模块基础配置文件说明详见 [mod_ai_rate_limit.conf](../../configuration/mod_ai_rate_limit/mod_ai_rate_limit.conf.md)。

## 规则配置

规则配置文件说明详见 [ai_rate_limit.data](../../configuration/mod_ai_rate_limit/ai_rate_limit.data.md)。
