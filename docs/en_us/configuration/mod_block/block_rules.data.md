# mod_block Rule Configuration

## Introduction

`block_rules.data` is the block rule configuration file of the `mod_block` module, used to configure block rules for each product line.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | Usually a timestamp, e.g. `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Block rules for each product line | Y | Keys are product line names | - |
| Config{k} | String | Product line name | Y | - | - |
| Config{v} | Array | Block rule list of the product line | Y | - | - |
| Config{v}[] | Object | A block rule | Y | - | - |
| Config{v}[].Cond | String | Rule matching condition | Y | Syntax see [Condition](../../condition/condition_grammar.md) | - |
| Config{v}[].Name | String | Rule name | Y | - | - |
| Config{v}[].Action | Object | Action after matching | Y | - | - |
| Config{v}[].Action.Cmd | String | Command to execute after matching | Y | Valid values see module actions | Must be `CLOSE` or `ALLOW` |
| Config{v}[].Action.Params | Array | Parameter list of the command | N | Parameters depend on the specific command; element type is String | - |

## Module Actions

| Action | Meaning |
| ------ | ------- |
| CLOSE | Close the connection |
| ALLOW | Accept the request |

## Configuration Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "global": [
            {
                "action": {
                    "cmd": "ALLOW",
                    "params": []
                },
                "cond": "req_host_in(\"n.example.org\") && req_path_prefix_in(\"/index/\", false) && req_query_key_in(\"space\")",
                "name": "example whiterule"
            }
        ],
        "example_product": [
            {
                "action": {
                    "cmd": "CLOSE",
                    "params": []
                },
                "name": "example rule",
                "cond": "req_path_in(\"/limit\", false)"
            }
        ]
    }
}
```
