# mod_userid Rule Configuration

## Configuration Introduction

`userid_rule.data` is the rule configuration file for the `mod_userid` module, used to configure the rules for adding user identification cookies to new users under each product line.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `2019-12-10184356` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | List of rules for the product | Y | - | - |
| Config{v}[] | Object | A rule | Y | - | - |
| Config{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| Config{v}[].Params | Object | Cookie parameters | Y | - | - |
| Config{v}[].Params.Name | String | Cookie name | Y | - | - |
| Config{v}[].Params.Domain | String | Cookie domain | N | - | - |
| Config{v}[].Params.Path | String | Cookie path | Y | - | - |
| Config{v}[].Params.MaxAge | Integer | Cookie max age | N | Unit is seconds | - |
| Config{v}[].Generator | String | User ID generator | N | E.g., `default` | - |

## Configuration Example

```json
{
    "Version": "2019-12-10184356",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_prefix_in(\"/abc\", true)",
                "Params": {
                     "Name": "bfe_userid_abc",
                     "Domain": "",
                     "Path": "/abc",
                     "MaxAge": 3153600
                 },
                 "Generator": "default"
            },
            {
                "Cond": "default_t()",
                "Params": {
                     "Name": "bfe_userid",
                     "Domain": "",
                     "Path": "/",
                     "MaxAge": 3153600
                 }
            }
        ]
    }
}
```
