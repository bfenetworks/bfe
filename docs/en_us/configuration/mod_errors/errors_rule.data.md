# mod_errors Rule Configuration

## Introduction

`errors_rule.data` is the rule configuration file of `mod_errors`, used to configure error response replacement or redirection rules for each product.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `20190101000000`; see [Version](../00-common.md#5-version) type definition | Type is [Version](../00-common.md#5-version) |
| Config | Object | Error rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Object | A list of error rules for the product | Y | - | - |
| Config{v}[] | Object | Error rule details | Y | - | - |
| Config{v}[].Cond | String | Condition for matching request or response | Y | Syntax see [Condition](../../condition/condition_grammar.md) | Must be a valid Condition expression |
| Config{v}[].Actions | Array | List of actions to execute after match | Y | - | - |
| Config{v}[].Actions[].Cmd | String | Command to execute after match | Y | Values: `RETURN` / `REDIRECT` | Must be `RETURN` or `REDIRECT` |
| Config{v}[].Actions[].Params | Array | List of parameters for the command | Y | `RETURN` requires 3 parameters; `REDIRECT` requires 1 parameter | Parameters must meet command requirements |
| Config{v}[].Actions[].Params[] | String | A single parameter | Y | - | - |

## Module Actions

| Action | Description | Parameters | Required | Supplementary Description | Validity Condition |
| ------ | ----------- | ---------- | -------- | ------------------------- | ------------------ |
| RETURN | Return response generated from specified static html | - | - | - | - |
| REDIRECT | Redirect to specified location | - | - | - | - |

## Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "res_code_in(\"404\")",
                "Actions": [
                    {
                        "Cmd": "RETURN",
                        "Params": [
                            "200", "text/html", "../conf/mod_errors/404.html"
                        ]
                    }
                ]
            },
            {
                "Cond": "res_code_in(\"500\")",
                "Actions": [
                    {
                        "Cmd": "REDIRECT",
                        "Params": [
                            "http://example.org/error.html"
                        ]
                    }
                ]
            }
        ]
    }
}
```
