# mod_access_pb3

## Introduction

mod_access_pb3 records access logs in Protocol Buffers v3 format.

## Module Configuration

### Description

Module configuration file: conf/mod_access_pb3/mod_access_pb3.conf

| Config Item | Description |
| ----------- | ----------- |
| Log.LogPrefix | String<br>Log file prefix (required) |
| Log.LogDir | String<br>Log file directory (required) |
| Log.RotateWhen | String<br>Log rotation time, e.g. NEXTHOUR, MIDNIGHT (required) |
| Log.BackupCount | Integer<br>Number of backup log files (must be > 0) |
| BasicConf.OpenDebug | Boolean<br>Whether to enable debug logs |

### Example

```ini
[Log]
LogPrefix = pb_access3
LogDir = /home/work/bfe/log
RotateWhen = NEXTHOUR
BackupCount = 2

[BasicConf]
OpenDebug = true
```

## Log Description

mod_access_pb3 outputs request logs and session logs in binary protobuf format. The log definitions are in [bfe-access-pb](https://github.com/bfenetworks/bfe-access-pb).

Request log tag (log_tag) rules:
- Normal request: `req_<product name>`
- Error request: `req_err_<product name>`
- Error request with empty product: `req_err_bfe`

## Metrics

| Metric | Description |
| ------ | ----------- |
| ALL_REQ_LOG_COUNT | Total count of request logs |
| ALL_SES_LOG_COUNT | Total count of session logs |
