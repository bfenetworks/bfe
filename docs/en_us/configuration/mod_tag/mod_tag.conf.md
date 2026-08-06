# mod_tag Basic Configuration

## Configuration Introduction

`mod_tag.conf` is the basic configuration file for the `mod_tag` module, used to specify the rule configuration file path.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration | Y | Default value is `mod_tag/tag_rule.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Log.OpenDebug | Boolean | Debug flag of module | N | Default value is `false` | - |

## Configuration Example

```ini
[Basic]
DataPath = mod_tag/tag_rule.data

[Log]
OpenDebug = true
```
