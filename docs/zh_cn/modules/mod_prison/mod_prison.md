# mod_prison

## 模块简介

mod_prison根据自定义的条件，限定单位时间用户的访问次数。

## 基础配置

模块基础配置文件说明详见 [mod_prison.conf](../../configuration/mod_prison/mod_prison.conf.md)。

## 规则配置

模块规则配置文件说明详见 [prison.data](../../configuration/mod_prison/prison.data.md)。

## 监控项

| 监控项      | 描述             |
| ----------- | ---------------- |
| ALL_CHECKED | 被检查的请求总数 |
| ALL_PRISON  | 被封禁的请求总数 |
