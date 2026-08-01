# mod_access 基础配置

## 配置简介

`mod_access.conf` 是 `mod_access` 模块的基础配置文件，用于配置请求日志和会话日志的输出路径、日志切割以及日志模板。

## 配置描述

| 配置项                   | 类型    | 参数含义                                                   | 必填 | 补充描述                                                         | 合法性条件                                              |
| ------------------------ | ------- | ---------------------------------------------------------- | ---- | ---------------------------------------------------------------- | ------------------------------------------------------- |
| Log.LogFile              | String  | 日志文件路径，用来将日志输出到单个文件中（不进行日志切割） | N    | 类型为 [FilePath](../00-common.md#3-文件路径filepath)               | -                                                       |
| Log.LogPrefix            | String  | 日志文件前缀名称                                           | N    | 通常与 `LogDir` 配合使用                                         | -                                                       |
| Log.LogDir               | String  | access 日志文件目录                                        | N    | 类型为 [FilePath](../00-common.md#3-文件路径filepath)               | -                                                       |
| Log.RotateWhen           | String  | 日志切割时间                                               | N    | 支持 `M` / `H` / `D` / `MIDNIGHT` / `NEXTHOUR`                   | 取值须为 `M`、`H`、`D`、`MIDNIGHT`、`NEXTHOUR` 之一     |
| Log.BackupCount          | Integer | 最大的日志存储数量                                         | N    | 日志轮转后保留的最大文件数                                       | 须为非负整数                                            |
| Template.RequestTemplate | String  | 请求日志模板                                               | N    | 支持以 `$` 开头的变量；支持内置模板 `COMMON`、`COMBINED`         | -                                                       |
| Template.SessionTemplate | String  | 会话日志模板                                               | N    | 支持以 `$` 开头的变量                                            | -                                                       |

* `RequestTemplate` / `SessionTemplate` 中 `$` 开头的代表变量，支持的变量列表详见模块文档中的"日志变量"章节说明。
* `RequestTemplate` 还支持以下内置模板：
  * `COMMON`：Common Log Format；等价于配置 `RequestTemplate = $host - - $request_time \"$request_line\" $status_code $res_len`
  * `COMBINED`：Combined Log Format；等价于配置 `RequestTemplate = $host - - $request_time \"$request_line\" $status_code $res_len \"${Referer}req_header\" \"${User-Agent}req_header\"`

## 配置示例

### 将日志保存到指定路径

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

### 将日志输出到标准输出

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
