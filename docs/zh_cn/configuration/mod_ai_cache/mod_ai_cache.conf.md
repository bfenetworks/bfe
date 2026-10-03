# mod_ai_cache 基础配置

## 配置简介

`mod_ai_cache.conf` 是 `mod_ai_cache` 模块的基础配置文件，用于指定缓存规则文件路径、缓存键前缀、默认 TTL、Redis 连接参数以及语义缓存（可选）的 embedding 服务与向量库连接信息。

`mod_ai_cache` 对符合规则的 AI 请求（OpenAI 协议）做两级缓存：

1. **Redis 精确匹配缓存**（默认开启）：请求中的用户问题与历史请求完全一致时命中；
2. **语义缓存**（可选）：精确未命中时，对问题文本做 embedding 后在向量库中检索近似问题，相似度超过阈值即返回缓存答案。

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
| Embedding.ServiceHost | String | embedding 服务地址 | 语义启用时 Y | OpenAI 兼容 `/v1/embeddings`（覆盖 OpenAI / Ollama / vLLM / SiliconFlow / DashScope 兼容模式等） | 非空字符串 |
| Embedding.ServicePort | Integer | embedding 服务端口 | 语义启用时 Y | Ollama 默认 11434；OpenAI 为 443（配合 UseHttps） | 1–65535 |
| Embedding.UseHttps | Boolean | 是否使用 HTTPS | N | 默认 false；true 时请求 `https://{host}:{port}/v1/embeddings` | - |
| Embedding.ApiKey | String | 访问凭证 | N | Bearer 方式传递；任何日志不输出明文 | - |
| Embedding.Model | String | embedding 模型名 | 语义启用时 Y | 请求体 `model` 字段 | 非空字符串 |
| Embedding.TimeoutMs | Integer | 单次调用超时 | N | 默认 500（毫秒），超时按未命中降级 | 1–5000 |
| Vector.Type | String | 向量库类型 | 语义启用时 Y | 目前仅支持 `chroma`（缺省按 chroma 处理） | 枚举：chroma |
| Vector.ServiceHost | String | 向量库地址 | 语义启用时 Y | Chroma 默认 8000 | 非空字符串 |
| Vector.ServicePort | Integer | 向量库端口 | 语义启用时 Y | - | 1–65535 |
| Vector.ApiKey | String | 访问凭证 | N | Chroma 默认无鉴权；Bearer 方式传递，日志不输出明文 | - |
| Vector.Collection | String | 集合名 | N | 默认 `ai_cache_semantic`；集合按 cosine 空间创建 | 非空字符串 |
| Vector.TimeoutMs | Integer | 单次向量操作超时 | N | 默认 300（毫秒） | 1–5000 |
| Vector.MaxQuestionBytes | Integer | 问题文本长度上限 | N | 默认 4096（字节），超限跳过语义检索（精确缓存不受影响） | 正整数 |
| Log.OpenDebug | Boolean | 是否开启 debug 日志 | N | 开启后访问日志记录 `ai_cache_key` | - |

> 说明：
> - Redis 出错时模块采用 **fail-open** 策略——查询失败按未命中处理、回写失败仅记日志，不影响主请求，因此不提供"出错拒绝"开关；
> - `[embedding]` / `[vector]` 两个配置段**整体可选、必须同时配置**：不配则语义能力全局关闭（纯精确缓存）；只配一边、取值非法或 provider 初始化失败（如 Chroma 不可达）时仅记 WARN 日志并全局关闭语义能力，**不影响启动**，精确缓存照常服务；
> - 请求处理顺序：Redis 精确 GET 优先，未命中且规则开启 `enableSemanticCache` 时才做 embedding + 向量检索；embedding 失败、向量查询失败、未过阈值均降级为普通 miss，主请求不受影响；
> - 语义命中与精确命中一样会跳过 `mod_ai_token_auth` 的配额/费用扣减（访问日志 `ai_cache_status` 分别为 `hit` / `hit_semantic`，语义命中额外记录 `ai_cache_semantic=true` 与 `ai_cache_similarity`）。

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

# 语义缓存（可选）：不配则只有 Redis 精确缓存
[embedding]
serviceHost = 127.0.0.1
servicePort = 11434
useHttps = false
apiKey =
model = nomic-embed-text
timeoutMs = 500

[vector]
type = chroma
serviceHost = 127.0.0.1
servicePort = 8000
apiKey =
collection = ai_cache_semantic
timeoutMs = 300
maxQuestionBytes = 4096

[log]
OpenDebug = false
```
