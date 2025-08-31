# mod_ai_token_auth

## Module Overview

mod_ai_token_auth supports API-key (token) authentication for LLM services. An API-key represents a token with certain access permissions and quotas for specific LLM services. This module checks the API-key carried in the request according to rules to determine whether the request is allowed to access the LLM service.

Request header carries the API-key:
```
Authorization: Bearer <api-key>
```

## Basic Configuration

### Configuration Description

Module configuration file: conf/mod_ai_token_auth/mod_ai_token_auth.conf

| Option              | Description                                        |
| ------------------- | ------------------------------------------------- |
| Basic.ProductRulePath      | String<br>File path for API-key declaration and rule configuration |
| redis.bns | String<br>BNS name of the Redis service. Redis is used to store API-key quota usage. |
| Log.OpenDebug       | Boolean<br>Enable debug logs<br>Default: False |

### Configuration Example

```ini
[basic]
ProductRulePath = mod_ai_token_auth/token_rule.data

[redis]
# bns addr
bns = BLB.ALB-redis

# timeout in ms
connectTimeout = 20
readTimeout = 20
writeTimeout = 20

# max idle connections
maxIdle = 20

[log]
OpenDebug = false
```

## Rule Configuration

### Configuration Description

| Option                | Description                                        |
| --------------------- | ------------------------------------------------- |
| Version | String<br>Configuration file version |
| Tokens | Object<br>API-key declarations for all product lines |
| Tokens{k} | String<br>Product line name|
| Tokens{v} | Object<br>All API-keys under a product line |
| Tokens{v}{k} | String<br>An API-key |
| Tokens{v}{v} | Object<br>An API-key declaration, data structure below. |
| Config | Object<br>API-key authentication rule configuration for all product lines |
| Config{k} | String<br>Product line name|
| Config{v} | Array<br>API-key authentication rule list under a product line |
| Config{v}[] | Object<br>API-key authentication rule |
| Config{v}[].Cond | String<br>Matching condition, syntax details in [Condition](../../condition/condition_grammar.md) |
| Config{v}[].Action | Object<br>Action. Only one action is supported: { "cmd": "CHECK_TOKEN" } |

API-key declaration data structure:
```
struct {
    Key            string           // API-key
    Status         int              // API-key status: 1 - Enabled; 2 - Disabled; 3 - Expired; 4 - Exhausted
    Name           string           // Name
    UpdateTime     int64            // Update time (Unix Time). Change means a new quota consumption cycle starts, recalculating UsedQuota.
    ExpiredTime    int64            // Expiry time (Unix Time). -1 means never expires
    RemainQuota    int64            // Total available quota (unit: token)
    UnlimitedQuota bool             // Unlimited quota or not
    Models         *string          // Allowed model list, multiple model names separated by commas
    Subnet         *string          // Allowed source IP subnet
}
```

### Configuration Example

```json
{
    "Config": {
        "example_product" :[
            {
                "cond": "default_t()",
                "action": {
                    "cmd": "CHECK_TOKEN"
                }
            }
        ]
    },
    "Tokens": {
        "example_product": {
            "TESTKEY": {
                "key": "TESTKEY",
                "status": 1,
                "name": "test",
                "expired_time": -1,
                "unlimited_quota": true
            }
        }
    },
    "Version": "20190101000000"
}
```
