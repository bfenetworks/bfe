# mod_markdown Basic Configuration

## Introduction

`mod_markdown.conf` is the basic configuration file of `mod_markdown`. It specifies the path of markdown rule file.

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.ProductRulePath | String | Path of markdown rule file | Y | Default `mod_markdown/markdown_rule.data` | Type is [FilePath](../00-common.md#3-filepath); file must exist and be readable |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default `False` | - |

## Example

```ini
[Basic]
ProductRulePath = mod_markdown.data

[Log]
OpenDebug = true
```
