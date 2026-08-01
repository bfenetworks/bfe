# mod_header Rule Configuration

## Introduction

`mod_header.data` is the rule configuration file of `mod_header`, used to configure header manipulation rules for each product.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `20190101000000`; see [Version](../00-common.md#5-version) type definition | Type is [Version](../00-common.md#5-version) |
| Config | Object | Header rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | List of header rules for the product | Y | - | - |
| Config{v}[] | Object | Header rule | Y | - | - |
| Config{v}[].Cond | String | Rule condition | Y | Syntax see [Condition](../../condition/condition_grammar.md) | Must be a valid Condition expression |
| Config{v}[].Last | Boolean | Whether to stop processing subsequent rules after a match | N | Defaults to `false` | - |
| Config{v}[].Actions | Array | List of actions to execute after match | Y | - | - |
| Config{v}[].Actions[].Cmd | String | Action name | Y | Valid values see module actions | - |
| Config{v}[].Actions[].Params | Array | Action parameter list | N | Parameters depend on the action; element type is String | - |

## Module Actions

| Action | Description | Parameters | Required | Supplementary Description | Validity Condition |
| -------------- | ---------------------- | ---------- | -------- | ------------------------- | ------------------ |
| REQ_HEADER_SET | Set request header | HeaderName, HeaderValue | - | - | - |
| REQ_HEADER_ADD | Add request header | HeaderName, HeaderValue | - | - | - |
| REQ_HEADER_DEL | Delete request header | HeaderName | - | - | - |
| REQ_HEADER_RENAME | Rename request header | OriginalHeaderName, NewHeaderName | - | - | - |
| RSP_HEADER_SET | Set response header | HeaderName, HeaderValue | - | - | - |
| RSP_HEADER_ADD | Add response header | HeaderName, HeaderValue | - | - | - |
| RSP_HEADER_DEL | Delete response header | HeaderName | - | - | - |
| RSP_HEADER_RENAME | Rename response header | OriginalHeaderName, NewHeaderName | - | - | - |
| REQ_HEADER_MOD | Modify request header | scheme_set/query_add, HeaderName, ... | - | - | - |
| RSP_HEADER_MOD | Modify response header | scheme_set/query_add, HeaderName, ... | - | - | - |
| REQ_COOKIE_SET | Set request Cookie | CookieName, CookieValue | - | - | - |
| REQ_COOKIE_DEL | Delete request Cookie | CookieName | - | - | - |
| RSP_COOKIE_SET | Set response Cookie | Name, Value, Domain, Path, Expires(RFC1123), MaxAge(int), HttpOnly(bool), Secure(bool) | - | - | - |
| RSP_COOKIE_DEL | Delete response Cookie | Name, Domain, Path | - | - | - |

## Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "cond": "req_path_prefix_in(\"/header\", false)",
                "actions": [
                    {
                        "cmd": "REQ_HEADER_SET",
                        "params": [
                            "X-Bfe-Log-Id",
                            "%bfe_log_id"
                        ]
                    },
                    {
                        "cmd": "REQ_HEADER_SET",
                        "params": [
                            "X-Bfe-Vip",
                            "%bfe_vip"
                        ]
                    },
                    {
                        "cmd": "RSP_HEADER_SET",
                        "params": [
                            "X-Proxied-By",
                            "bfe"
                        ]
                    }
                ],
                "last": true
            }
        ]
    }
}
```
