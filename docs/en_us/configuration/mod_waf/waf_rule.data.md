# mod_waf Rule Configuration

## Introduction

`waf_rule.data` is the rule configuration file of the `mod_waf` module, used to configure WAF detection rules for each product line.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | Usually a timestamp, e.g. `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | WAF rules for each product line | Y | Keys are product line names | - |
| Config{k} | String | Product line name | Y | - | - |
| Config{v} | Array | List of WAF rules under the product line | Y | - | - |
| Config{v}[] | Object | A WAF rule | Y | - | - |
| Config{v}[].Cond | String | Condition to match the request | Y | Syntax see [Condition](../../condition/condition_grammar.md) | - |
| Config{v}[].BlockRules | []String | List of rule names that block directly on hit | N | At least one of `BlockRules` or `CheckRules` must be configured | Rule names must be supported by the current module |
| Config{v}[].CheckRules | []String | List of rule names that log only on hit | N | At least one of `BlockRules` or `CheckRules` must be configured | Rule names must be supported by the current module |

## Supported WAF Rules

| Rule Name | Meaning |
| --------- | ------- |
| RuleBashCmd | Bash command injection detection |

## Configuration Example

```json
{
    "Version": "2019-12-10184356",
    "Config": {
        "example_product": [
            {
                "Cond": "default_t()",
                "BlockRules": [
                    "RuleBashCmd"
                ]
            }
        ]
    }
}
```
