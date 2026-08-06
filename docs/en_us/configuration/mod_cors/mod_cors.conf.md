# mod_cors Basic Configuration

## Introduction

`mod_cors.conf` is the basic configuration file of the `mod_cors` module, used to specify the rule configuration file path.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of the rule configuration file | Y | Default `mod_cors/cors_rule.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default `False` | - |

## Configuration Example

```ini
[Basic]
DataPath = mod_cors/cors_rule.data

[Log]
OpenDebug = false
```
