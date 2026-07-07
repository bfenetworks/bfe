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
| Basic.CacheSize | Integer<br>Cache size for Sticky mode, default is 10000 |
| Log.OpenDebug | Boolean<br>Debug flag of module |

### Example

```ini
[Basic]
DataPath = mod_session_sticky/session_sticky.data
CacheSize = 10000

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

1. **Encode**: When a request first arrives, BFE selects a backend and stores the mapping between stickyid (obtained from Cookie) and backend information in the cache.
2. **Decode**: Subsequent requests from the client carry the same stickyid (obtained from Cookie, request header, or URL parameter). BFE looks up the corresponding backend information from the cache and routes the request to the corresponding backend.

### Cookie Renewal Mechanism

When the remaining validity period of the Cookie is less than `RenewWindow`, BFE will reset the Cookie to extend its validity period. This ensures that users do not lose session sticky status due to Cookie expiration during long sessions.

### Mask Code Compatibility

The module supports primary and standby mask codes. If decryption with the primary mask code fails, it will try the standby mask code. This is useful when you need to change the mask code without interrupting existing sessions.