# mod_access

## Introduction

mod_access writes request logs and session logs in the specified format.

## Module Configuration

### Description
  conf/mod_access/mod_access.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Log.LogPrefix | String<br>filename prefix for log |
| Log.LogDir | String<br>directory of log files |
| Log.RotateWhen | String<br>inteval to rotate log file |
| Log.BackupCount | Integer<br>max number of rotated log files |
| Template.RequestTemplate | String<br>template of request log |
| Template.SessionTemplate | String<br>template of session log |

### Example

```
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
