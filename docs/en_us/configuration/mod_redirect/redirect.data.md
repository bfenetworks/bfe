# mod_redirect Rule Configuration

## Introduction

`redirect.data` is the rule configuration file of `mod_redirect`.

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp string, e.g., `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Redirect rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | A ordered list of redirect rules | Y | - | - |
| Config{v}[] | Object | A redirect rule | Y | - | - |
| Config{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) | - |
| Config{v}[].Actions | Array | A ordered list of redirect actions | Y | - | - |
| Config{v}[].Actions[] | Object | A redirect action | Y | - | - |
| Config{v}[].Actions[].Cmd | String | Name of redirect action | Y | Valid values see module actions | - |
| Config{v}[].Actions[].Params | Object | Parameters of redirect action | N | Depends on specific action; URL parameters support `=`-prefixed expressions, see below | - |
| Config{v}[].Status | Integer | HTTP status code | N | - | Valid HTTP redirect status code |

## Actions

| Action | Description |
| ------ | ----------- |
| URL_SET | Redirect to specified URL |
| URL_FROM_QUERY | Redirect to URL parsed from specified query in request |
| URL_PREFIX_ADD | Redirect to URL concatenated by specified prefix and the original URL |
| SCHEME_SET | Redirect to the original URL but with scheme changed. supported scheme: http&#124;https |

### Action Param Expressions

`Params[0]` of `URL_SET` and `URL_PREFIX_ADD` (target URL / URL prefix) supports
inline expressions prefixed with `=`; the query key of `URL_FROM_QUERY` and the
scheme value of `SCHEME_SET` remain literals. See
[ActionParam](../00-common.md#10-actionparam) for syntax and error handling.

Example (build the redirect URL from the request host):

```json
{
    "Cmd": "URL_SET",
    "Params": [
        "=\"https://\" + ReqHost() + \"/v2\""
    ]
}
```

## Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_prefix_in(\"/redirect\", false)",
                "Actions": [
                    {
                        "Cmd": "URL_SET",
                        "Params": ["https://example.org"]
                    }
                ],
                "Status": 301
            }
        ]
    }
}
```
