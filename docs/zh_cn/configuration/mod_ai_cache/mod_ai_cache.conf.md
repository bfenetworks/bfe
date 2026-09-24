# mod_ai_cache 基础配置

## 配置简介

`mod_ai_cache.conf` 是 `mod_ai_cache` 模块的基础配置文件，用于指定缓存规则文件路径、缓存键前缀、默认 TTL 以及 Redis 连接参数等。

`mod_ai_cache` 对符合规则的 AI 请求（OpenAI 协议）做 **Redis 精确匹配缓存**：仅当请求中的用户问题与历史请求完全一致时命中缓存，直接返回答案，不再调用上游模型。本模块为简化版实现，只支持精确字符串匹配，不支持语义/向量缓存。

## 配置描述

| 配置项 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| ------ | ---- | -------- | ---- | -------- | ---------- |
| Basic.ProductRulePath | String | 缓存规则文件路径 | Y | - | 文件须存在且可读 |
| Basic.CacheKeyPrefix | String | 缓存键前缀 | N | 默认值为 `ai_cache`；缓存键最终格式为 `{prefix}:{key_id}:{question_hash}` | 非空字符串 |
| Basic.DefaultCacheTTL | Integer | 默认缓存 TTL（秒） | N | 规则未配置 `cacheTTL` 时生效，默认 3600 | 非负整数 |
| Redis.Bns | String | Redis 代理 BNS 地址 | N | - | 非空字符串 |
| Redis.ConnectTimeout | Integer | 连接 Redis 超时时间 | N | 单位：毫秒 | 必须大于 0 |
| Redis.ReadTimeout | Integer | 读取 Redis 超时时间 | N | 单位：毫秒 | 必须大于 0 |
| Redis.WriteTimeout | Integer | 写入 Redis 超时时间 | N | 单位：毫秒 | 必须大于 0 |
| Redis.MaxIdle | Integer | Redis 连接池最大空闲连接数 | N | - | 非负整数 |
| Redis.MaxActive | Integer | Redis 连接池最大活跃连接数 | N | 0 表示无限制 | 非负整数 |
| Redis.Password | String | Redis 密码 | N | 未设置时忽略；不会写入访问日志 | - |
| Log.OpenDebug | Boolean | 是否开启 debug 日志 | N | 开启后访问日志记录 `ai_cache_key` | - |

> 说明：Redis 出错时模块采用 **fail-open** 策略——查询失败按未命中处理、回写失败仅记日志，不影响主请求，因此不提供"出错拒绝"开关。

## 配置示例

```ini
[basic]
ProductRulePath = ../conf/mod_ai_cache/mod_ai_cache_rule.data
CacheKeyPrefix = ai_cache
DefaultCacheTTL = 3600

[redis]
bns = BLB.ALB-redis
connectTimeout = 20
readTimeout = 20
writeTimeout = 20
maxIdle = 20

[log]
OpenDebug = false
```
