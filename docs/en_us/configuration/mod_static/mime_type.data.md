# mod_static MIME Configuration

## Configuration Introduction

`mime_type.data` is the MIME type mapping configuration file for the `mod_static` module.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | File extension to MIME type mapping | Y | - | - |
| Config{k} | String | File extension | Y | Must start with `.` | - |
| Config{v} | String | MIME type | Y | - | - |

## Configuration Example

```json
{
    "Config": {
        ".avi": "video/x-msvideo",
        ".doc": "application/msword"
    },
    "Version": "20190101000000"
}
```
