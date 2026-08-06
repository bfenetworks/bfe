# mod_cors Rule Configuration

## Introduction

`cors_rule.data` is the rule configuration file of the `mod_cors` module.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | Usually a timestamp, e.g. `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | CORS rules for each product line | Y | Keys are product line names | - |
| Config{k} | String | Product line name | Y | - | - |
| Config{v} | Array | CORS rule list of the product line | Y | - | - |
| Config{v}[] | Object | A CORS rule | Y | - | - |
| Config{v}[].Cond | String | Rule condition | Y | Syntax see [Condition](../../condition/condition_grammar.md) | - |
| Config{v}[].AccessControlAllowOrigins | Array | List of origins allowed to access cross-origin resources | N | `"%origin"` means allow any domain and use the request `Origin` value; `"*"` means allow all domains for requests without credentials | - |
| Config{v}[].AccessControlAllowCredentials | Boolean | Whether the browser is allowed to expose the response to the page | N | - | - |
| Config{v}[].AccessControlExposeHeaders | Array | List of response headers that the client is allowed to access | N | - | - |
| Config{v}[].AccessControlAllowMethods | Array | Used for preflight requests; list of methods allowed by the client in the actual request | N | - | - |
| Config{v}[].AccessControlAllowHeaders | Array | Used for preflight requests; list of request headers allowed by the client in the actual request | N | - | - |
| Config{v}[].AccessControlMaxAge | Integer | Used for preflight requests; indicates how long (in seconds) the preflight request result can be cached | N | `-1` means disable cache | Greater than or equal to `-1` |

## Configuration Example

```json
{
    "Version": "cors_rule.data.version",
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"example.org\")",
                "AccessControlAllowOrigins": ["%origin"],
                "AccessControlAllowCredentials": true,
                "AccessControlExposeHeaders": ["X-Custom-Header"],
                "AccessControlAllowMethods": ["HEAD","GET","POST","PUT","DELETE","OPTIONS","PATCH"],
                "AccessControlAllowHeaders": ["X-Custom-Header"],
                "AccessControlMaxAge": -1
            }
        ]
    }
}
```
