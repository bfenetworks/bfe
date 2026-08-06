# mod_ai_rate_limit Basic Configuration

## Introduction

`mod_ai_rate_limit.conf` is the basic configuration file of the `mod_ai_rate_limit` module, used to specify the rate limit rule file path, Redis connection parameters, etc.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.ProductRulePath | String | Path of the rate limit rule file | Y | - | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Basic.IsRejectOnRedisError | Boolean | Whether to reject requests when Redis error occurs | N | Default `false` | - |
| Redis.Bns | String | Redis proxy BNS address | N | - | Non-empty string |
| Redis.ConnectTimeout | Integer | Connection timeout to Redis | N | Unit: milliseconds | Must be greater than 0 |
| Redis.ReadTimeout | Integer | Read timeout from Redis | N | Unit: milliseconds | Must be greater than 0 |
| Redis.WriteTimeout | Integer | Write timeout to Redis | N | Unit: milliseconds | Must be greater than 0 |
| Redis.MaxIdle | Integer | Max idle connections in Redis connection pool | N | - | Non-negative integer |
| Redis.MaxActive | Integer | Max active connections in Redis connection pool | N | `0` means unlimited | Non-negative integer |
| Redis.Password | String | Redis password | N | Ignored if not set | - |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | - | - |

## Configuration Example

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
