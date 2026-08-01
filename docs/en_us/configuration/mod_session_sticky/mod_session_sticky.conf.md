# mod_session_sticky Basic Configuration

## Configuration Introduction

`mod_session_sticky.conf` is the basic configuration file for the `mod_session_sticky` module, used to specify the rule configuration file path, cache type, and Redis related configuration.

## Configuration Description

### Basic Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration | Y | Default value is `mod_session_sticky/session_sticky.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Basic.CacheSize | Integer | Cache size for Sticky mode | N | Default value is `10000`; only effective when `CacheType` is `local` | Non-negative integer |
| Basic.CacheType | String | Cache type | N | Default value is `local`; valid values are `local` (LRU cache) or `redis` (Redis distributed cache) | Value range is `local`, `redis` |
| Log.OpenDebug | Boolean | Debug flag of module | N | - | - |

### Redis Configuration

When `CacheType` is set to `redis`, the following Redis parameters are required:

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Redis.Bns | String | BNS service name | Conditional | Required when `CacheType` is `redis` | - |
| Redis.ConnectTimeout | Integer | Connect timeout in milliseconds | Conditional | Required when `CacheType` is `redis` | Positive integer |
| Redis.ReadTimeout | Integer | Read timeout in milliseconds | Conditional | Required when `CacheType` is `redis` | Positive integer |
| Redis.WriteTimeout | Integer | Write timeout in milliseconds | Conditional | Required when `CacheType` is `redis` | Positive integer |
| Redis.MaxIdle | Integer | Max idle connections | Conditional | Required when `CacheType` is `redis` | Non-negative integer |
| Redis.MaxActive | Integer | Max active connections | Conditional | Required when `CacheType` is `redis`; `0` means unlimited | Non-negative integer |
| Redis.Password | String | Redis password | Conditional | Required when `CacheType` is `redis`; empty string means no password | - |
| Redis.ExpireSeconds | Integer | Cache expire time in seconds | Conditional | Required when `CacheType` is `redis` | Positive integer |

## Configuration Example

### Local Cache Mode

```ini
[Basic]
DataPath = mod_session_sticky/session_sticky.data
CacheSize = 10000
CacheType = local

[Log]
OpenDebug = true
```

### Redis Cache Mode

```ini
[Basic]
DataPath = mod_session_sticky/session_sticky.data
CacheType = redis

[Redis]
Bns = redis.service
ConnectTimeout = 1000
ReadTimeout = 1000
WriteTimeout = 1000
MaxIdle = 10
MaxActive = 100
Password = 
ExpireSeconds = 3600

[Log]
OpenDebug = true
```
