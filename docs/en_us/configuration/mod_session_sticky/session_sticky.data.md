# mod_session_sticky Rule Configuration

## Configuration Introduction

`session_sticky.data` is the rule configuration file for the `mod_session_sticky` module.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `2024-01-01 00:00:00` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | A list of rules | Y | - | - |
| Config{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| Config{v}[].Type | String | Session sticky type | N | Default value is `Cookie`; valid values are `Cookie` or `Sticky` | Value range is `Cookie`, `Sticky` |
| Config{v}[].CookieKey | String | Cookie name | N | Default value is `bfe_ssbl` | - |
| Config{v}[].Domain | String | Cookie domain | N | - | - |
| Config{v}[].Path | String | Cookie path | N | - | - |
| Config{v}[].MaxAge | Integer | Cookie max age in seconds | N | Default value is `3600` | Non-negative integer |
| Config{v}[].MaskCode | String | Primary mask code for encrypting Cookie value | N | Default value is `defaultmask`; used for encrypting Cookie in Cookie mode | Length must be ≥ 4 if configured |
| Config{v}[].StandbyMaskCode | String | Standby mask code for decrypting when primary fails | N | - | Length must be ≥ 4 |
| Config{v}[].Header | String | Header name to get stickyid in Sticky mode | N | - | - |
| Config{v}[].URIParam | String | URL parameter name to get stickyid in Sticky mode | N | - | - |
| Config{v}[].StickyRequestField | String | JSON field name in request body for sticky ID | N | e.g., `previous_response_id`, used for OpenAI-compatible interfaces | - |
| Config{v}[].StickyResponseField | String | JSON field name in response body for sticky ID | N | e.g., `response_id`, used for OpenAI-compatible interfaces | - |
| Config{v}[].Secure | Boolean | Cookie Secure attribute | N | Default value is `false` | - |
| Config{v}[].HttpOnly | Boolean | Cookie HttpOnly attribute | N | Default value is `false` | - |
| Config{v}[].RenewWindow | Integer | Cookie renew window in seconds | N | When remaining validity is less than this value, Cookie will be reset; default value is half of MaxAge | Non-negative integer |

## Configuration Example

### Cookie Mode Example

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

### Sticky Mode Example

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
