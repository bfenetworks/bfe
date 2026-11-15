# mod_access_pb3 基础配置

## 配置简介

`mod_access_pb3.conf` 是 `mod_access_pb3` 模块的基础配置文件，用于配置以 Protocol Buffers v3 格式输出访问日志的文件路径及日志轮转策略。

## 配置描述

| 配置项            | 类型    | 参数含义         | 必填 | 补充描述                                           | 合法性条件                                              |
| ----------------- | ------- | ---------------- | ---- | -------------------------------------------------- | ------------------------------------------------------- |
| Log.LogPrefix     | String  | 日志文件前缀     | Y    | 与 `LogDir` 配合使用                               | -                                                       |
| Log.LogDir        | String  | 日志文件目录     | Y    | 类型为 [FilePath](../00-common.md#3-文件路径filepath) | -                                                       |
| Log.RotateWhen    | String  | 日志轮转时间     | Y    | 支持 `NEXTHOUR`、`MIDNIGHT` 等                     | 取值须为 `M`、`H`、`D`、`MIDNIGHT`、`NEXTHOUR` 之一     |
| Log.BackupCount   | Integer | 日志备份数量     | N    | 日志轮转后保留的最大文件数                         | 若配置，取值须大于 0                                    |
| BasicConf.OpenDebug | Boolean | 是否开启 debug 日志 | N  | 默认值 `False`                                     | -                                                       |
| BasicConf.DataConfPath | String | 动态配置文件 `mod_access_pb.data` 的路径 | N | 用于按白名单采集请求头/响应头等动态配置；缺省不启用（等价全段缺省，不采集）。相对路径基于 conf root 解析。**配置的文件不存在时模块不报错**，按缺省行为运行；文件支持热加载（reload 接口 `/reload/mod_access_pb3`） | 类型为 [FilePath](../00-common.md#3-文件路径filepath) |

## 配置示例

```ini
[Log]
LogPrefix = pb_access3
LogDir = /home/work/bfe/log
RotateWhen = NEXTHOUR
BackupCount = 2

[BasicConf]
OpenDebug = true
DataConfPath = ../conf/mod_access_pb3/mod_access_pb.data
```
