# mod_markdown

## Introduction

mod_markdown renders markdown files to HTML in real time and returns the result to the client.

## Module Configuration

### Description

Module configuration file: conf/mod_markdown/mod_markdown.conf

| Config Item | Description |
| ----------- | ----------- |
| Basic.ProductRulePath | String<br>Path of markdown rule file<br>Default mod_markdown/markdown_rule.data |
| Log.OpenDebug | Boolean<br>Whether to enable debug logs<br>Default False |

### Example

```ini
[Basic]
ProductRulePath = mod_markdown.data

[Log]
OpenDebug = true
```

## Rule Configuration

### Description

Rule configuration file: markdown_rule.data

| Config Item | Description |
| ----------- | ----------- |
| Version | String<br>Version of config file |
| Config | Object<br>Markdown rendering rules for each product |
| Config{k} | String<br>Product name |
| Config{v} | Object<br>List of rules under the product |
| Config{v}[] | Object<br>Detailed rule information |
| Config{v}[].Cond | String<br>Condition to match the request, see [Condition](../../condition/condition_grammar.md) |

### Example

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

## Metrics

| Metric | Description |
| ------ | ----------- |
| REQ_TOTAL | Total count of requests |
| REQ_MARK_DOWN_RULE_HIT | Count of requests hitting markdown rule |
| RSP_RENDER_SUCCESS | Count of successful rendering responses |
| RSP_RENDER_FAILURE | Count of failed rendering responses |
| RSP_RENDER_IGNORE | Count of ignored rendering responses |
| ERR_COUNT_READ_FAIL | Count of read failures |
| ERR_COUNT_RENDER_FAIL | Count of render failures |
