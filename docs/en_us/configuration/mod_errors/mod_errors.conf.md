# mod_errors Basic Configuration

## Introduction

`mod_errors.conf` is the basic configuration file of `mod_errors`, used to specify the path of the error rule configuration file and log options.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration file | N | Defaults to `mod_errors/mod_errors.data`; see [FilePath](../00-common.md#3-filepath) type definition | Type is [FilePath](../00-common.md#3-filepath); empty uses default |
| Log.OpenDebug | Boolean | Whether to enable debug log | N | Defaults to `False` | - |

## Example

```ini
[Basic]
DataPath = mod_errors/errors_rule.data

[Log]
OpenDebug = false
```
