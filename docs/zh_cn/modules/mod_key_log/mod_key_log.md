# mod_key_log

## 模块简介

mod_key_log以NSS key log格式记录TLS会话密钥, 便于基于第三方工具（例如wireshark) 解密分析TLS加密流量，方便问题诊断分析

关于NSS key log详细格式说明，参见:
https://developer.mozilla.org/en-US/docs/Mozilla/Projects/NSS/Key_Log_Format

## 基础配置

### 配置描述

模块配置文件: conf/mod_key_log/mod_key_log.conf

| 配置项                | 描述                                        |
| ----------------------| ------------------------------------------- |
| Log.LogFile | String<br>日志文件路径，用来将日志输出到单个文件中（不进行日志切割） |
| Log.LogPrefix | String<br>日志文件前缀名称 |
| Log.LogDir | String<br>日志文件目录 |
| Log.RotateWhen | String<br>日志切割时间，支持 M/H/D/MIDNIGHT/NEXTHOUR |
| Log.BackupCount | Integer<br>最大的日志存储数量 |

### 配置示例

#### 将日志保存到指定目录

```ini
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

#### 将日志输出到标准输出

```ini
[Log]
# filename prefix for log 
LogFile = /dev/stdout
```
