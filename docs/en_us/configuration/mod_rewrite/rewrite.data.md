# mod_rewrite Rule Configuration

## Configuration Introduction

`rewrite.data` is the rule configuration file for the `mod_rewrite` module.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Rewrite rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | A ordered list of rewrite rules | Y | - | - |
| Config{v}[] | Object | A rewrite rule | Y | - | - |
| Config{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| Config{v}[].Actions | Array | A ordered list of rewrite actions | Y | - | - |
| Config{v}[].Actions[] | Object | A rewrite action | Y | - | - |
| Config{v}[].Actions[].Cmd | String | Name of rewrite action | Y | See Module Actions for valid values | - |
| Config{v}[].Actions[].Params | Array | Parameters of rewrite action | N | Depends on the specific action | - |
| Config{v}[].Last | Boolean | Stop checking remaining rules if true | N | Default value is `false` | - |

## Module Actions

| Action | Description |
| ------ | ----------- |
| HOST_SET | Set host to specified value |
| HOST_SET_FROM_PATH_PREFIX | Set host to specified path prefix |
| HOST_SUFFIX_REPLACE | Replace suffix of host |
| PATH_SET | Set path to specified value |
| PATH_PREFIX_ADD | Add prefix to original path |
| PATH_PREFIX_TRIM | Trim prefix from original path |
| QUERY_ADD | Add query |
| QUERY_DEL | Delete query |
| QUERY_DEL_ALL_EXCEPT | Delete all queries except specified queries |
| QUERY_RENAME | Rename query |

## Configuration Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_prefix_in(\"/rewrite\", false)",
                "Actions": [
                    {
                        "Cmd": "PATH_PREFIX_ADD",
                        "Params": [
                            "/bfe/"
                        ]
                    }
                ],
                "Last": true
            }
        ]
    }
}
```
