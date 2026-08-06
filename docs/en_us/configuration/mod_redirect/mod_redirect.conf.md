# mod_redirect Basic Configuration

## Introduction

`mod_redirect.conf` is the basic configuration file of `mod_redirect`. It specifies the path of rule configuration file.

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration file | Y | Default `mod_redirect/redirect.data` | Type is [FilePath](../00-common.md#3-filepath); file must exist and be readable |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default `False` | - |

## Example

```ini
[Basic]
DataPath = mod_redirect/redirect.data

[Log]
OpenDebug = false
```
