# mod_static Basic Configuration

## Configuration Introduction

`mod_static.conf` is the basic configuration file for the `mod_static` module, used to specify the static file rule configuration file path, MIME configuration file path, and compression switch.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration | Y | Default value is `mod_static/static_rule.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Basic.MimeTypePath | String | Path of MIME configuration | Y | Default value is `mod_static/mime_type.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Basic.EnableCompress | Boolean | Whether to enable static file compression | N | Default value is `False` | - |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default value is `False` | - |

## Configuration Example

```ini
[Basic]
DataPath = mod_static/static_rule.data
MimeTypePath = mod_static/mime_type.data
EnableCompress = false

[Log]
OpenDebug = false
```
