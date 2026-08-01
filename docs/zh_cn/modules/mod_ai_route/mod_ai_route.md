# mod_ai_route

## 模块简介

mod_ai_route 用于根据 AI 路由规则，将 AI 请求路由到不同的后端集群和模型。支持基于 apikey、entity 和 global 三种类型的路由表。

## 基础配置

模块基础配置文件说明详见 [mod_ai_route.conf](../../configuration/mod_ai_route/mod_ai_route.conf.md)。

## 规则配置

模块规则配置文件说明详见 [ai_route.data](../../configuration/mod_ai_route/ai_route.data.md)。

## 监控项

| 监控项         | 描述                     |
| -------------- | ------------------------ |
| REQ_TOTAL      | 请求总数                 |
| REQ_HIT_APIKEY | 命中 apikey 路由的请求数 |
| REQ_HIT_ENTITY | 命中 entity 路由的请求数 |
| REQ_HIT_GLOBAL | 命中 global 路由的请求数 |
| REQ_MISS       | 未命中路由的请求数       |
| REQ_FALLBACK   | 命中 fallback 的请求数   |
