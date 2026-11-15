# mod_access_pb3 Basic Configuration

## Introduction

`mod_access_pb3.conf` is the basic configuration file of the `mod_access_pb3` module, used to configure the output path, rotation policy, and other settings for access logs in Protocol Buffers v3 format.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Log.LogPrefix | String | Log file prefix | Y | Usually used together with `LogDir` | - |
| Log.LogDir | String | Directory of log files | Y | Type is [FilePath](../00-common.md#3-filepath) | - |
| Log.RotateWhen | String | Interval to rotate log file | Y | Supports `NEXTHOUR`, `MIDNIGHT`, etc. | Value must be one of `M`, `H`, `D`, `MIDNIGHT`, `NEXTHOUR` |
| Log.BackupCount | Integer | Max number of rotated log files | N | Number of log files retained after rotation | If configured, value must be greater than 0 |
| BasicConf.OpenDebug | Boolean | Whether to enable debug logs | N | Default `False` | - |
| BasicConf.DataConfPath | String | Path of the dynamic configuration file `mod_access_pb.data` | N | Used for dynamic configuration such as request/response header whitelists; when absent, dynamic config is not enabled (identical to all sections missing). Relative paths are resolved against the conf root. **The module does not fail when the configured file is absent** and falls back to default behavior; the file supports hot reload (reload API `/reload/mod_access_pb3`) | Type is [FilePath](../00-common.md#3-filepath) |

## Configuration Example

```ini
[Log]
LogPrefix = pb_access3
LogDir = /home/work/bfe/log
RotateWhen = NEXTHOUR
BackupCount = 2

[BasicConf]
OpenDebug = true
DataConfPath = ../conf/mod_access_pb3/mod_access_pb.data
```
