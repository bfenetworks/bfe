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
| redis.connectTimeout | Integer<br>Connect timeout in milliseconds |
| redis.readTimeout | Integer<br>Read timeout in milliseconds |
| redis.writeTimeout | Integer<br>Write timeout in milliseconds |
| redis.maxIdle | Integer<br>Max idle connections |
| redis.maxActive | Integer<br>Max active connections (0 means unlimited) |
| redis.password | String<br>Redis password (optional) |
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

# max active connections
maxActive = 100

# redis password (optional)
password = 

[log]
OpenDebug = false
```

## Rule Configuration

### Configuration Description

| Option                | Description                                        |
| --------------------- | ------------------------------------------------- |
| Version | String<br>Configuration file version |
| QuotaPlans | Object<br>Quota plan declarations for all product lines |
| QuotaPlans{k} | String<br>Product line name|
| QuotaPlans{v} | Array<br>Quota plan list under a product line |
| QuotaPlans{v}[] | Object<br>Quota plan, data structure below |
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

Quota Plan data structure:
```
struct {
    Id          string           // Quota plan ID
    Unlimited   bool             // Unlimited quota or not
    PassNoQuota bool             // Allow request when quota is insufficient
    RedisKey    string           // Redis key for storing quota
    CreateTime  int64            // Create time (Unix Time)
    ExpiredTime int64            // Expiry time (Unix Time). -1 means never expires
    Quota       int64            // Total quota (unit: token)
    ResetMode   int              // Reset mode: 0 - non-periodic; 1 - periodic quota package
}
```

API-key declaration data structure:
```
struct {
    Key            string           // API-key
    Status         int              // API-key status: 1 - Enabled; 2 - Disabled; 3 - Expired; 4 - Exhausted
    Name           string           // Name
    UpdateTime     int64            // Update time (Unix Time). Change means a new quota consumption cycle starts, recalculating UsedQuota.
    ExpiredTime    int64            // Expiry time (Unix Time). -1 means never expires
    UnlimitedQuota bool             // Unlimited quota or not
    Models         *string          // Allowed model list, multiple model names separated by commas
    BlockModels    *string          // Blocked model list, multiple model names separated by commas
    Subnet         *string          // Allowed source IP subnet
    Tags           []ApikeyTag      // API-key tag list
    QuotaPlans     []string         // Associated quota plan ID list
}
```

### Configuration Example

```json
{
    "Version": "20190101000000",
    "QuotaPlans": {
        "example_product": [
            {
                "id": "daily_quota",
                "unlimited": false,
                "pass_no_quota": false,
                "redis_key": "ai:quota:daily_quota",
                "create_time": 1672531200,
                "expired_time": -1,
                "quota": 100000,
                "reset_mode": 1
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
                "unlimited_quota": false,
                "allow_models": "model_a,model_b",
                "block_models": "model_c",
                "subnet": "192.168.0.0/24",
                "tags": [
                    {"key": "department", "value": "engineering"}
                ],
                "quota_plans": ["daily_quota"]
            }
        }
    },
    "Config": {
        "example_product": [
            {
                "cond": "default_t()",
                "action": {
                    "cmd": "CHECK_TOKEN"
                }
            }
        ]
    }
}
```

## Working Principle

### Token Authentication Flow

1. **Request Entry**: When a request arrives, the module checks if it matches the authentication rule
2. **Token Validation**: Extracts the API-key from the `Authorization` header of the request and validates its validity (status, expiration time, etc.)
3. **Model Permission Check**: Verifies whether the requested model is in the allowed list and not in the blocked list
4. **IP Subnet Check**: Verifies whether the request source IP is within the allowed subnet range
5. **Quota Check**: Checks if the associated quota plans have sufficient quota
6. **Quota Deduction**: After the request completes, extracts the token usage from the response body and deducts the corresponding quota

### Monitoring Metrics

| Metric Name | Type | Description |
| ----------- | ---- | ----------- |
| ReqTotal | Counter | Total number of requests |
| ReqAuth | Counter | Number of requests triggering authentication |
| ReqAuthFail | Counter | Number of authentication failures |
