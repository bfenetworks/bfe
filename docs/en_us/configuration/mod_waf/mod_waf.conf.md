# mod_waf Basic Configuration

## Introduction

`mod_waf.conf` is the basic configuration file of the `mod_waf` module, used to specify the WAF rule file path and log output settings.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.ProductRulePath | String | Path of the WAF rule file | Y | Default is `mod_waf/waf_rule.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Log.LogFile | String | Path of log file; outputs logs to a single file without rotation | N | Type is [FilePath](../00-common.md#3-filepath) | Mutually exclusive with `LogPrefix`, `LogDir`, `RotateWhen`, and `BackupCount`; cannot be configured at the same time |
| Log.LogPrefix | String | Log file prefix | N | Usually used together with `LogDir` | Required when `LogFile` is not configured |
| Log.LogDir | String | Directory of log files | N | Type is [DirPath](../00-common.md#4-dirpath) | Required when `LogFile` is not configured |
| Log.RotateWhen | String | Interval to rotate log file | N | Supports `M` / `H` / `D` / `MIDNIGHT` / `NEXTHOUR` | Required when `LogFile` is not configured; value must be one of `M`, `H`, `D`, `MIDNIGHT`, `NEXTHOUR` |
| Log.BackupCount | Integer | Max number of rotated log files | N | Number of log files retained after rotation | Required when `LogFile` is not configured; must be greater than 0 |

## Configuration Example

```ini
[Basic]
ProductRulePath = mod_waf/waf_rule.data

[Log]
LogPrefix = waf
LogDir = ../log
RotateWhen = NEXTHOUR
BackupCount = 24
```
