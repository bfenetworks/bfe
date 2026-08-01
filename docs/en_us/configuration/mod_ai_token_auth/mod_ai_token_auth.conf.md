# mod_ai_token_auth Basic Configuration

## Configuration Introduction

`mod_ai_token_auth.conf` is the basic configuration file for the `mod_ai_token_auth` module, used to specify the API-key rule configuration file path, Redis connection information, and debug log switch.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.ProductRulePath | String | File path for API-key declaration and rule configuration | N | Default value is `mod_ai_token_auth/token_rule.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Redis.Bns | String | BNS name of the Redis service | Y | Redis is used to store API-key quota usage | Must be a valid Redis service address |
| Redis.ConnectTimeout | Integer | Connect timeout in milliseconds | Y | - | Must be greater than 0 |
| Redis.ReadTimeout | Integer | Read timeout in milliseconds | Y | - | Must be greater than 0 |
| Redis.WriteTimeout | Integer | Write timeout in milliseconds | Y | - | Must be greater than 0 |
| Redis.MaxIdle | Integer | Max idle connections | Y | - | Must be greater than or equal to 0 |
| Redis.MaxActive | Integer | Max active connections | Y | 0 means unlimited | Must be greater than or equal to 0 |
| Redis.Password | String | Redis password | N | Authentication is disabled if not configured | - |
| Log.OpenDebug | Boolean | Debug flag of module | N | Default value is `false` | - |

## Configuration Example

```ini
[Basic]
ProductRulePath = mod_ai_token_auth/token_rule.data

[Redis]
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

[Log]
OpenDebug = false
```
