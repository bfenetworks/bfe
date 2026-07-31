# mod_access_pb3

## 模块简介

mod_access_pb3 用于以 Protocol Buffers v3 格式记录访问日志。

## 基础配置

### 配置描述

模块配置文件: conf/mod_access_pb3/mod_access_pb3.conf

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Log.LogPrefix         | String<br>日志文件前缀（必填） |
| Log.LogDir            | String<br>日志文件目录（必填） |
| Log.RotateWhen        | String<br>日志轮转时间，如 NEXTHOUR、MIDNIGHT（必填） |
| Log.BackupCount       | Integer<br>日志备份数量（必须 > 0） |
| BasicConf.OpenDebug   | Boolean<br>是否开启 debug 日志 |

### 配置示例

```ini
[Log]
LogPrefix = pb_access3
LogDir = /home/work/bfe/log
RotateWhen = NEXTHOUR
BackupCount = 2

[BasicConf]
OpenDebug = true
```

## 日志说明

mod_access_pb3 以二进制 protobuf 格式输出请求日志和会话日志，日志定义参见 [bfe-access-pb](https://github.com/bfenetworks/bfe-access-pb)。

请求日志标签（log_tag）规则：
- 正常请求：`req_<product name>`
- 错误请求：`req_err_<product name>`
- 错误请求且 product 为空：`req_err_bfe`

## 监控项

| 监控项         | 描述                     |
| -------------- | ------------------------ |
| ALL_REQ_LOG_COUNT | 请求日志总数             |
| ALL_SES_LOG_COUNT | 会话日志总数             |
