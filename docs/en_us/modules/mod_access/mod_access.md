# mod_access

## Introduction

mod_access writes request logs and session logs in the specified format.

## Module Configuration

### Description

  conf/mod_access/mod_access.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Log.LogFile | String<br>Set file path of log for saving to a single file without rotation |
| Log.LogPrefix | String<br>Filename prefix for log |
| Log.LogDir | String<br>Directory of log files |
| Log.RotateWhen | String<br>Interval to rotate log file |
| Log.BackupCount | Integer<br>Max number of rotated log files |
| Template.RequestTemplate | String<br>Template of request log |
| Template.SessionTemplate | String<br>Template of session log |

### Example

#### Save log to a directory

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

#### Save log to a stdout

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
