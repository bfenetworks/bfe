# mod_prison Basic Configuration

## Introduction

`mod_prison.conf` is the basic configuration file of `mod_prison`. It specifies the path of rule configuration file.

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.ProductRulePath | String | Path of rule configuration file | Y | Default `mod_prison/prison.data` | Type is [FilePath](../00-common.md#3-filepath); file must exist and be readable |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default `False` | - |

## Example

```ini
[Basic]
ProductRulePath = mod_prison/prison.data

[Log]
OpenDebug = false
```
