# mod_ai_rate_limit

## 模块简介

mod_ai_rate_limit 用于对 AI 请求进行限流。支持基于 Redis 的分布式限流，可按产品、apikey 等维度配置 TPM（每分钟 Token 数）、RPM（每分钟请求数）和最大并发数限制。

## 基础配置

### 配置描述

模块配置文件: conf/mod_ai_rate_limit/mod_ai_rate_limit.conf

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Basic.ProductRulePath | String<br>限流规则文件路径（必填） |
| Basic.IsRejectOnRedisError | Boolean<br>Redis 出错时是否拒绝请求 |
| Redis.Bns             | String<br>Redis 代理 BNS 地址 |
| Redis.ConnectTimeout  | Integer<br>连接 Redis 超时时间，单位毫秒（必须 > 0） |
| Redis.ReadTimeout     | Integer<br>读取 Redis 超时时间，单位毫秒（必须 > 0） |
| Redis.WriteTimeout    | Integer<br>写入 Redis 超时时间，单位毫秒（必须 > 0） |
| Redis.MaxIdle         | Integer<br>Redis 连接池最大空闲连接数 |
| Redis.MaxActive       | Integer<br>Redis 连接池最大活跃连接数，0 表示无限制 |
| Redis.Password        | String<br>Redis 密码，未设置时忽略 |
| Log.OpenDebug         | Boolean<br>是否开启 debug 日志 |

### 配置示例

```ini
[basic]
ProductRulePath = ../data/mod_ai_rate_limit/ai_rate_limit.data

[redis]
bns = BLB.ALB-redis
connectTimeout = 20
readTimeout = 20
writeTimeout = 20
maxIdle = 20

[log]
OpenDebug = true
```

## 规则配置

### 配置描述

规则配置文件: ai_rate_limit.data

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>各产品线的限流规则 |
| Config{k} | String<br>产品线名称 |
| Config{v} | []Object<br>产品线下的限流规则列表 |
| Config{v}[].cond | String<br>匹配请求的条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].hit_action | Object<br>命中限流后的动作 |
| Config{v}[].hit_action.cmd | String<br>动作指令，如 PASS |
| RateLimitPolicies | Object<br>限流策略集合 |
| RateLimitPolicies{k} | String<br>策略 ID |
| RateLimitPolicies{v} | Object<br>策略详细信息 |
| RateLimitPolicies{v}.name | String<br>策略名称 |
| RateLimitPolicies{v}.enabled | Boolean<br>是否启用 |
| RateLimitPolicies{v}.models | []String<br>该策略适用的模型列表 |
| RateLimitPolicies{v}.rules | Object<br>限流规则详情 |
| RateLimitPolicies{v}.rules.tpm | []Object<br>TPM 限流规则列表 |
| RateLimitPolicies{v}.rules.tpm[].name | String<br>规则名称 |
| RateLimitPolicies{v}.rules.tpm[].window_minutes | Integer<br>时间窗口，单位分钟 |
| RateLimitPolicies{v}.rules.tpm[].max_tokens | Integer<br>窗口内最大 Token 数 |
| RateLimitPolicies{v}.rules.tpm[].step_minutes | Integer<br>步长，单位分钟 |
| RateLimitPolicies{v}.rules.tpm[].models | []String<br>该规则适用的模型列表 |
| RateLimitPolicies{v}.rules.rpm | []Object<br>RPM 限流规则列表 |
| RateLimitPolicies{v}.rules.rpm[].name | String<br>规则名称 |
| RateLimitPolicies{v}.rules.rpm[].window_minutes | Integer<br>时间窗口，单位分钟 |
| RateLimitPolicies{v}.rules.rpm[].max_requests | Integer<br>窗口内最大请求数 |
| RateLimitPolicies{v}.rules.rpm[].burst | Integer<br>突发请求数 |
| RateLimitPolicies{v}.rules.rpm[].models | []String<br>该规则适用的模型列表 |
| RateLimitPolicies{v}.rules.max_concurrency | Integer<br>最大并发数 |
| ApikeyRateLimitPolicyBindings | Object<br>apikey 到策略 ID 列表的绑定关系 |
| ApikeyRateLimitPolicyBindings{k} | String<br>apikey |
| ApikeyRateLimitPolicyBindings{v} | []String<br>该 apikey 绑定的策略 ID 列表 |

### 配置示例

```json
{
    "Version": "1.0",
    "Config": {
        "AI_product": [
            {
                "cond": "default_t()",
                "hit_action": {
                    "cmd": "PASS"
                }
            }
        ]
    },
    "RateLimitPolicies": {
        "rlp-0001": {
            "name": "ratelimitX",
            "enabled": true,
            "rules": {
                "tpm": [
                    {"name":"abc0", "window_minutes": 1, "max_tokens": 10000, "step_minutes": 1, "models": ["gpt-4", "gpt-4o"]},
                    {"name":"abc1", "window_minutes": 10, "max_tokens": 50000, "step_minutes": 1, "models": ["gpt-3.5"]},
                    {"name":"abc2", "window_minutes": 60, "max_tokens": 200000, "step_minutes": 5}
                ],
                "rpm": [
                    {"name":"abc0", "window_minutes": 1, "max_requests": 100, "burst":1, "models": ["gpt-4"]}
                ],
                "max_concurrency": 50
            }
        }
    },
    "ApikeyRateLimitPolicyBindings": {
        "ak-2v8x9k3m7p": ["rlp-0001"]
    }
}
```
