# mod_trace Rule Configuration

## Configuration Introduction

`trace_rule.data` is the rule configuration file for the `mod_trace` module, used to configure whether to enable distributed tracing for each product.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Trace rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | A list of trace rules | Y | - | - |
| Config{v}[] | Object | A trace rule | Y | - | - |
| Config{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| Config{v}[].Enable | Boolean | Enable trace | Y | - | - |

## Configuration Example

```json
{
  "Version": "20200218210000",
  "Config": {
    "example_product": [
       {
         "Cond": "req_host_in(\"example.org\")",
         "Enable": true
       }
    ]
  }
}
```
