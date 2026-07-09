# mod_session_sticky

## Introduction

mod_session_sticky implements session sticky functionality, ensuring that requests from the same user are always routed to the same backend server during the session lifetime.

This module supports two session sticky modes:

- **Cookie mode**: Backend information is encrypted and stored in a Cookie. The client carries this Cookie with each request, and BFE routes the request to the corresponding backend based on the Cookie content.
- **Sticky mode**: Maintains a mapping between stickyid and backend through a cache mechanism. Supports getting stickyid from Cookie, request header, or URL parameter.

## Module Configuration

### Description

conf/mod_session_sticky/mod_session_sticky.conf

| Config Item | Description |
| ----------- | ----------- |
| Basic.DataPath | String<br>Path of rule configuration |
| Basic.CacheSize | Integer<br>Cache size for Sticky mode, default is 10000 (only effective when CacheType is "local") |
| Basic.CacheType | String<br>Cache type, "local" (LRU cache) or "redis" (Redis distributed cache), default is "local" |
| Log.OpenDebug | Boolean<br>Debug flag of module |

### Redis Configuration

When `CacheType` is set to "redis", the following Redis parameters are required:

| Config Item | Description |
| ----------- | ----------- |
| Redis.Bns | String<br>BNS service name |
| Redis.ConnectTimeout | Integer<br>Connect timeout in milliseconds |
| Redis.ReadTimeout | Integer<br>Read timeout in milliseconds |
| Redis.WriteTimeout | Integer<br>Write timeout in milliseconds |
| Redis.MaxIdle | Integer<br>Max idle connections |
| Redis.MaxActive | Integer<br>Max active connections (0 means unlimited) |
| Redis.Password | String<br>Redis password |
| Redis.ExpireSeconds | Integer<br>Cache expire time in seconds |

### Example

#### Local Cache Mode

```ini
[Basic]
DataPath = mod_session_sticky/session_sticky.data
CacheSize = 10000
CacheType = local

[Log]
OpenDebug = true
```

#### Redis Cache Mode

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

## Rule Configuration

### Description

conf/mod_session_sticky/session_sticky.data

| Config Item | Description |
| ----------- | ----------- |
| Version | String<br>Version of config file |
| Config | Object<br>Rules for each product |
| Config{k} | String<br>Product name |
| Config{v} | Object<br>A list of rules |
| Config[v][].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config[v][].Type | String<br>Session sticky type, "Cookie" or "Sticky", default is "Cookie" |
| Config[v][].CookieKey | String<br>Cookie name, default is "bfe_ssbl" |
| Config[v][].Domain | String<br>Cookie domain |
| Config[v][].Path | String<br>Cookie path |
| Config[v][].MaxAge | Integer<br>Cookie max age in seconds, default is 3600 |
| Config[v][].MaskCode | String<br>Primary mask code for encrypting Cookie value, minimum length 4 |
| Config[v][].StandbyMaskCode | String<br>Standby mask code for decrypting when primary fails, minimum length 4 |
| Config[v][].Header | String<br>Header name to get stickyid in Sticky mode |
| Config[v][].URIParam | String<br>URL parameter name to get stickyid in Sticky mode |
| Config[v][].StickyRequestField | String<br>JSON field name in request body for sticky ID (e.g., previous_response_id), used for OpenAI-compatible interfaces |
| Config[v][].StickyResponseField | String<br>JSON field name in response body for sticky ID (e.g., response_id), used for OpenAI-compatible interfaces |
| Config[v][].Secure | Boolean<br>Cookie Secure attribute, default is false |
| Config[v][].HttpOnly | Boolean<br>Cookie HttpOnly attribute, default is false |
| Config[v][].RenewWindow | Integer<br>Cookie renew window in seconds. When remaining validity is less than this value, Cookie will be reset. Default is half of MaxAge |

### Example

#### Cookie Mode Example

```json
{
    "Version": "2024-01-01 00:00:00",
    "Config": {
        "example_product": [
            {
                "Cond": "default_t()",
                "Type": "Cookie",
                "CookieKey": "bfe_ssbl",
                "Domain": ".example.com",
                "Path": "/",
                "MaxAge": 3600,
                "MaskCode": "my_secret_mask_code",
                "StandbyMaskCode": "backup_mask_code",
                "Secure": true,
                "HttpOnly": true,
                "RenewWindow": 1800
            }
        ]
    }
}
```

#### Sticky Mode Example

```json
{
    "Version": "2024-01-01 00:00:00",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_prefix_in(\"/api\", true)",
                "Type": "Sticky",
                "CookieKey": "JSESSIONID",
                "Header": "X-Sticky-Id",
                "URIParam": "sticky_id"
            }
        ]
    }
}
```

## Working Principle

### Cookie Mode

1. **Encode**: When a request first arrives without session sticky information, BFE selects a backend and encrypts the backend information (address, port, subcluster) into a Cookie, which is returned to the client.
2. **Decode**: Subsequent requests from the client carry this Cookie. BFE decrypts the Cookie content, retrieves the backend information, and routes the request to the corresponding backend.

### Sticky Mode

1. **Encode**: When a request first arrives, BFE selects a backend and stores the mapping between stickyid (obtained from Cookie or JSON response body) and backend information in the cache.
2. **Decode**: Subsequent requests from the client carry the same stickyid (obtained from Cookie, request header, URL parameter, or JSON request body). BFE looks up the corresponding backend information from the cache and routes the request to the corresponding backend.

### Cache Types

The module supports two cache types:

- **local**: Uses LRU cache to store the mapping between stickyid and backend. Suitable for single-node deployment or scenarios where session sharing across nodes is not required.
- **redis**: Uses Redis to store the mapping. Suitable for multi-node deployment scenarios to ensure session sharing across different nodes.

### JSON Request/Response Fields

For OpenAI-compatible interfaces and similar scenarios, the module supports extracting stickyid from JSON request and response bodies:

- **StickyRequestField**: Extracts stickyid from JSON request body, for example, from the `previous_response_id` field.
- **StickyResponseField**: Extracts stickyid from JSON response body, for example, from the `response_id` field.

Extraction priority (Decode phase): Cookie > Header > URIParam > StickyRequestField

Extraction priority (Encode phase): Cookie > StickyResponseField

### Cookie Renewal Mechanism

When the remaining validity period of the Cookie is less than `RenewWindow`, BFE will reset the Cookie to extend its validity period. This ensures that users do not lose session sticky status due to Cookie expiration during long sessions.

### Mask Code Compatibility

The module supports primary and standby mask codes. If decryption with the primary mask code fails, it will try the standby mask code. This is useful when you need to change the mask code without interrupting existing sessions.