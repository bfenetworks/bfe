# mod_markdown Rule Configuration

## Introduction

`markdown_rule.data` is the rule configuration file of `mod_markdown`.

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp string, e.g., `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Markdown rendering rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | List of rules under the product | Y | - | - |
| Config{v}[] | Object | Detailed rule information | Y | - | - |
| Config{v}[].Cond | String | Condition to match the request | Y | See [Condition](../../condition/condition_grammar.md) | - |

## Example

```json
{
    "Version": "123",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_in(\"/md\", false)"
            }
        ]
    }
}
```
