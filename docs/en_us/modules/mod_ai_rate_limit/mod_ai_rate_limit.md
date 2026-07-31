# mod_ai_rate_limit

## Introduction

mod_ai_rate_limit performs rate limiting on AI requests. It supports distributed rate limiting based on Redis, and can configure TPM (tokens per minute), RPM (requests per minute), and max concurrency limits by dimensions such as product and apikey.

## Module Configuration

### Description

Module configuration file: conf/mod_ai_rate_limit/mod_ai_rate_limit.conf

| Config Item | Description |
| ----------- | ----------- |
| Basic.ProductRulePath | String<br>Path of rate limit rule file (required) |
| Basic.IsRejectOnRedisError | Boolean<br>Whether to reject requests when Redis error occurs |
| Redis.Bns | String<br>Redis proxy BNS address |
| Redis.ConnectTimeout | Integer<br>Connection timeout to Redis in milliseconds (must be > 0) |
| Redis.ReadTimeout | Integer<br>Read timeout from Redis in milliseconds (must be > 0) |
| Redis.WriteTimeout | Integer<br>Write timeout to Redis in milliseconds (must be > 0) |
| Redis.MaxIdle | Integer<br>Max idle connections in Redis connection pool |
| Redis.MaxActive | Integer<br>Max active connections in Redis connection pool, 0 means unlimited |
| Redis.Password | String<br>Redis password, ignored if not set |
| Log.OpenDebug | Boolean<br>Whether to enable debug logs |

### Example

```ini
[Basic]
ProductRulePath = ../data/mod_ai_rate_limit/ai_rate_limit.data

[Redis]
bns = BLB.ALB-redis
connectTimeout = 20
readTimeout = 20
writeTimeout = 20
maxIdle = 20

[Log]
OpenDebug = true
```

## Rule Configuration

### Description

Rule configuration file: ai_rate_limit.data

| Config Item | Description |
| ----------- | ----------- |
| Version | String<br>Version of config file |
| Config | Object<br>Rate limit rules for each product |
| Config{k} | String<br>Product name |
| Config{v} | []Object<br>List of rate limit rules under the product |
| Config{v}[].cond | String<br>Condition to match the request, see [Condition](../../condition/condition_grammar.md) |
| Config{v}[].hit_action | Object<br>Action when rate limit is hit |
| Config{v}[].hit_action.cmd | String<br>Action command, e.g. PASS |
| RateLimitPolicies | Object<br>Collection of rate limit policies |
| RateLimitPolicies{k} | String<br>Policy ID |
| RateLimitPolicies{v} | Object<br>Detailed policy information |
| RateLimitPolicies{v}.name | String<br>Policy name |
| RateLimitPolicies{v}.enabled | Boolean<br>Whether the policy is enabled |
| RateLimitPolicies{v}.models | []String<br>List of models the policy applies to |
| RateLimitPolicies{v}.rules | Object<br>Rate limit rule details |
| RateLimitPolicies{v}.rules.tpm | []Object<br>List of TPM rate limit rules |
| RateLimitPolicies{v}.rules.tpm[].name | String<br>Rule name |
| RateLimitPolicies{v}.rules.tpm[].window_minutes | Integer<br>Time window in minutes |
| RateLimitPolicies{v}.rules.tpm[].max_tokens | Integer<br>Max token count within the window |
| RateLimitPolicies{v}.rules.tpm[].step_minutes | Integer<br>Step size in minutes |
| RateLimitPolicies{v}.rules.tpm[].models | []String<br>List of models the rule applies to |
| RateLimitPolicies{v}.rules.rpm | []Object<br>List of RPM rate limit rules |
| RateLimitPolicies{v}.rules.rpm[].name | String<br>Rule name |
| RateLimitPolicies{v}.rules.rpm[].window_minutes | Integer<br>Time window in minutes |
| RateLimitPolicies{v}.rules.rpm[].max_requests | Integer<br>Max request count within the window |
| RateLimitPolicies{v}.rules.rpm[].burst | Integer<br>Burst request count |
| RateLimitPolicies{v}.rules.rpm[].models | []String<br>List of models the rule applies to |
| RateLimitPolicies{v}.rules.max_concurrency | Integer<br>Max concurrency |
| ApikeyRateLimitPolicyBindings | Object<br>Binding from apikey to list of policy IDs |
| ApikeyRateLimitPolicyBindings{k} | String<br>apikey |
| ApikeyRateLimitPolicyBindings{v} | []String<br>List of policy IDs bound to the apikey |

### Example

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
