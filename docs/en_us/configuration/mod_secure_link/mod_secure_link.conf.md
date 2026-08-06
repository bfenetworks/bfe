# mod_secure_link Basic Configuration

## Configuration Introduction

`mod_secure_link.conf` is the basic configuration file for the `mod_secure_link` module, used to specify the rule configuration file path.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration | Y | Default value is `mod_secure_link/secure_link_rule.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Log.OpenDebug | Boolean | Debug flag of module | N | - | - |

## Configuration Example

```ini
[Basic]
DataPath = ./mod_secure_link/secure_link_rule.data

[Log]
OpenDebug = true
```
