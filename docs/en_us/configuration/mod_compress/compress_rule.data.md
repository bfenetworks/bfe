# mod_compress Rule Configuration

## Introduction

`compress_rule.data` is the rule configuration file of the `mod_compress` module.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | Usually a timestamp, e.g. `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Compress rules for each product line | Y | Keys are product line names | - |
| Config{k} | String | Product line name | Y | - | - |
| Config{v} | Array | Compress rule list of the product line | Y | - | - |
| Config{v}[] | Object | A compress rule | Y | - | - |
| Config{v}[].Cond | String | Rule condition | Y | Syntax see [Condition](../../condition/condition_grammar.md) | - |
| Config{v}[].Action | Object | Action after matching | Y | - | - |
| Config{v}[].Action.Cmd | String | Name of the action | Y | Valid values see module actions | - |
| Config{v}[].Action.Quality | Integer | Compression level | N | Depends on the specific compression algorithm | - |
| Config{v}[].Action.FlushSize | Integer | Compression buffer size | N | Unit is byte | Positive integer |

## Module Actions

| Action | Meaning |
| ------ | ------- |
| GZIP | Compress response using gzip method |
| BROTLI | Compress response using brotli method |

## Configuration Example

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "GZIP",
                    "Quality": 9,
                    "FlushSize": 512
                }
            }
        ]
    },
    "Version": "20190101000000"
}
```
