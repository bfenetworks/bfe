# mod_compress Basic Configuration

## Introduction

`mod_compress.conf` is the basic configuration file of the `mod_compress` module, used to specify the rule configuration file path.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.ProductRulePath | String | Path of the rule configuration file | Y | Default `mod_compress/compress_rule.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default `False` | - |

## Configuration Example

```ini
[Basic]
ProductRulePath = mod_compress/compress_rule.data

[Log]
OpenDebug = false
```
