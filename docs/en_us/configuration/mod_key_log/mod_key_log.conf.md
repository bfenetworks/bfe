# mod_key_log Basic Configuration

## Introduction

`mod_key_log.conf` is the basic configuration file of `mod_key_log`. It specifies the path of rule data file and the NSS key log output mode.

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration file | Y | - | Type is [FilePath](../00-common.md#3-filepath); file must exist and be readable |
| Log.LogFile | String | Log file path without rotation | N | Mutually exclusive with `LogPrefix` / `LogDir` / `RotateWhen` / `BackupCount` | Type is [FilePath](../00-common.md#3-filepath); file must be writable |
| Log.LogPrefix | String | Filename prefix for log | N | Used together with `LogDir` / `RotateWhen` / `BackupCount` | - |
| Log.LogDir | String | Directory of log files | N | Used together with `LogPrefix` / `RotateWhen` / `BackupCount` | Type is [DirPath](../00-common.md#4-dirpath); directory must exist and be readable |
| Log.RotateWhen | String | Interval to rotate log file | N | Supports `M` / `H` / `D` / `MIDNIGHT` / `NEXTHOUR` | Value must be a valid log rotate interval |
| Log.BackupCount | Integer | Max number of rotated log files | N | - | Value must be > 0 |

## Example

### Save log to a directory

```ini
[Basic]
DataPath = mod_key_log/key_log.data

[Log]
# filename prefix for log 
LogPrefix = key

# log directory 
LogDir = ../log

# interval to rotate logs: M/H/D/MIDNIGHT/NEXTHOUR
# - M: minute
# - H: hour
# - D: day
RotateWhen = H 

# max number of rotated log files
BackupCount = 3
```

### Output log to stdout

```ini
[Basic]
DataPath = mod_key_log/key_log.data

[Log]
# filename prefix for log 
LogFile = /dev/stdout
```
