# mod_header Basic Configuration

## Introduction

`mod_header.conf` is the basic configuration file of `mod_header`, used to specify the path of the header rule configuration file and related options.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration file | Y | Defaults to `mod_header/mod_header.data`; see [FilePath](../00-common.md#3-filepath) type definition | Type is [FilePath](../00-common.md#3-filepath); file must exist and be readable |
| Basic.DisableDefaultHeader | Boolean | Whether to disable default header | N | Defaults to `False` | - |
| Log.OpenDebug | Boolean | Whether to enable debug log | N | Defaults to `False` | - |

## Example

```ini
[Basic]
DataPath = mod_header/mod_header.data
DisableDefaultHeader = false

[Log]
OpenDebug = false
```
