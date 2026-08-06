# mod_auth_jwt Basic Configuration

## Configuration Introduction

`mod_auth_jwt.conf` is the basic configuration file for the `mod_auth_jwt` module, used to specify the rule configuration file path.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration | Y | - | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Log.OpenDebug | Boolean | Debug flag of module | N | Default value is `false` | - |

## Configuration Example

```ini
[Basic]
DataPath = mod_auth_jwt/auth_jwt_rule.data

[Log]
OpenDebug = false
```
