# mod_static Rule Configuration

## Configuration Introduction

`static_rule.data` is the rule configuration file for the `mod_static` module.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Static rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | A ordered list of static rules | Y | - | - |
| Config{v}[] | Object | A static rule | Y | - | - |
| Config{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| Config{v}[].Action | Object | A static action | Y | - | - |
| Config{v}[].Action.Cmd | String | Name of static action | Y | Valid value is `BROWSE` | Value range is `BROWSE` |
| Config{v}[].Action.Params | Array | Parameters of static action | Y | - | - |
| Config{v}[].Action.Params[0] | String | Root directory location | Y | - | Type is [DirPath](../00-common.md#4-dirpath) |
| Config{v}[].Action.Params[1] | String | Default static file name | Y | - | - |

## Configuration Example

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "BROWSE",
                    "Params": [
                        "./",
                        "index.html"
                    ]
                }
            }
        ]
    },
    "Version": "20190101000000"
}
```
