# mod_rewrite Basic Configuration

## Configuration Introduction

`mod_rewrite.conf` is the basic configuration file for the `mod_rewrite` module, used to specify the rule configuration file path.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration | Y | Default value is `mod_rewrite/rewrite.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default value is `False` | - |

## Configuration Example

```ini
[Basic]
DataPath = mod_rewrite/rewrite.data

[Log]
OpenDebug = false
```
