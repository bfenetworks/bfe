# mod_auth_request Rule Configuration

## Configuration Introduction

`auth_request_rule.data` is the rule configuration file for the `mod_auth_request` module, used to configure request authentication rules by product line.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Request auth rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | A list of request auth rules | Y | - | - |
| Config{v}[] | Object | A request auth rule | Y | - | - |
| Config{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| Config{v}[].Enable | Boolean | Whether enable request auth rule | Y | - | - |

## Configuration Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_in(\"/auth_request\", false)",
                "Enable": true
            }
        ]
    }
}
```
