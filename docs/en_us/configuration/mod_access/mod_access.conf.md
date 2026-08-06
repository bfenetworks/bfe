# mod_access Basic Configuration

## Configuration Introduction

`mod_access.conf` is the basic configuration file for the `mod_access` module, used to configure the output path, rotation policy, and templates for request logs and session logs.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Log.LogFile | String | File path for saving logs to a single file without rotation | N | Type is [FilePath](../00-common.md#3-filepath) | - |
| Log.LogPrefix | String | Filename prefix for log | N | Usually used together with `LogDir` | - |
| Log.LogDir | String | Directory of log files | N | Type is [FilePath](../00-common.md#3-filepath) | - |
| Log.RotateWhen | String | Interval to rotate log file | N | Supports `M` / `H` / `D` / `MIDNIGHT` / `NEXTHOUR` | Value must be one of `M`, `H`, `D`, `MIDNIGHT`, `NEXTHOUR` |
| Log.BackupCount | Integer | Max number of rotated log files | N | Number of log files retained after rotation | Must be a non-negative integer |
| Template.RequestTemplate | String | Template of request log | N | Supports variables starting with `$`; supports built-in templates `COMMON` and `COMBINED` | - |
| Template.SessionTemplate | String | Template of session log | N | Supports variables starting with `$` | - |

* In `RequestTemplate` / `SessionTemplate`, values starting with `$` are variables. For the list of supported variables, see the "Log Variables" section in the module documentation.
* `RequestTemplate` also supports the following built-in templates:
  * `COMMON`: Common Log Format; equivalent to `RequestTemplate = $host - - $request_time \"$request_line\" $status_code $res_len`
  * `COMBINED`: Combined Log Format; equivalent to `RequestTemplate = $host - - $request_time \"$request_line\" $status_code $res_len \"${Referer}req_header\" \"${User-Agent}req_header\"`

## Configuration Example

### Save log to a directory

```ini
[Log]
# filename prefix for log
LogPrefix = access

# access log directory
LogDir =  ../log

# log rotate interval: M/H/D/MIDNIGHT/NEXTHOUR
RotateWhen = NEXTHOUR

# max number of rotated log files
BackupCount = 2

[Template]
# template of request log
RequestTemplate = "REQUEST_LOG $time clientip: $remote_addr serverip: $server_addr host: $host product: $product user_agent: ${User-Agent}req_header status: $status_code error: $error"

# template of session log
SessionTemplate = "SESSION_LOG  $time clientip: $ses_clientip start_time: $ses_start_time end_time: $ses_end_time overhead: $ses_overhead read_total: $ses_read_total write_total: $ses_write_total keepalive_num: $ses_keepalive_num error: $ses_error"
```

### Save log to stdout

```ini
[Log]
# file path for log
LogFile = /dev/stdout

[Template]
# template of request log
RequestTemplate = "REQUEST_LOG $time clientip: $remote_addr serverip: $server_addr host: $host product: $product user_agent: ${User-Agent}req_header status: $status_code error: $error"

# template of session log
SessionTemplate = "SESSION_LOG  $time clientip: $ses_clientip start_time: $ses_start_time end_time: $ses_end_time overhead: $ses_overhead read_total: $ses_read_total write_total: $ses_write_total keepalive_num: $ses_keepalive_num error: $ses_error"
```
