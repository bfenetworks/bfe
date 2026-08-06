# mod_tag Rule Configuration

## Configuration Introduction

`tag_rule.data` is the rule configuration file for the `mod_tag` module.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `20200218210000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Tag rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | List of tag rules for the product | Y | - | - |
| Config{v}[] | Object | A tag rule | Y | - | - |
| Config{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| Config{v}[].Param.TagName | String | Tag name | Y | - | - |
| Config{v}[].Param.TagValue | String | Tag value | Y | - | - |
| Config{v}[].Last | Boolean | Stop checking remaining rules if true | N | Default value is `false` | - |

## Configuration Example

```json
{
  "Version": "20200218210000",
  "Config": {
    "example_product": [
      {
        "Cond": "req_host_in(\"example.org\")",
        "Param": {
          "TagName": "tag",
          "TagValue": "bfe"
        },
        "Last": false
      }
    ]
  }
}
```
