# mod_key_log 基础配置

## 配置简介

`mod_key_log.conf` 是 `mod_key_log` 模块的基础配置文件，用于指定规则数据文件路径及 NSS key log 日志输出方式。

## 配置描述

| 配置项          | 类型    | 参数含义                 | 必填 | 补充描述                                                     | 合法性条件                                                   |
| --------------- | ------- | ------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Basic.DataPath  | String  | 规则配置文件路径         | Y    | -                                                            | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Log.LogFile     | String  | 日志文件路径（不进行日志切割） | N | 与 `LogPrefix` / `LogDir` / `RotateWhen` / `BackupCount` 互斥 | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须可写 |
| Log.LogPrefix   | String  | 日志文件前缀名称         | N    | 与 `LogDir` / `RotateWhen` / `BackupCount` 组合使用          | -                                                            |
| Log.LogDir      | String  | 日志文件目录             | N    | 与 `LogPrefix` / `RotateWhen` / `BackupCount` 组合使用       | 类型为 [DirPath](../00-common.md#4-目录路径dirpath)；目录须存在且可读 |
| Log.RotateWhen  | String  | 日志切割时间             | N    | 支持 `M` / `H` / `D` / `MIDNIGHT` / `NEXTHOUR`               | 取值须为有效的日志切割时间                                   |
| Log.BackupCount | Integer | 最大的日志存储数量       | N    | -                                                            | 取值须 `> 0`                                                 |

## 配置示例

### 将日志保存到指定目录

```ini
[Basic]
DataPath = mod_key_log/key_log.json

[Log]
# filename prefix for log
LogPrefix = key

# log directory
LogDir = ../log

# interval to rotate logs: M/H/D/MIDNIGHT/NEXTHOUR
RotateWhen = H

# max number of rotated log files
BackupCount = 3
```

### 将日志输出到标准输出

```ini
[Basic]
DataPath = mod_key_log/key_log.json

[Log]
# filename prefix for log
LogFile = /dev/stdout
```
