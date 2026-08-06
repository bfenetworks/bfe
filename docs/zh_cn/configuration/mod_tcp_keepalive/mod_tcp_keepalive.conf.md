# mod_tcp_keepalive 基础配置

## 配置简介

`mod_tcp_keepalive.conf` 是 `mod_tcp_keepalive` 模块的基础配置文件，用于指定规则配置文件路径及日志配置。

## 配置描述

| 配置项         | 类型    | 参数含义           | 必填 | 补充描述 | 合法性条件                                      |
| -------------- | ------- | ------------------ | ---- | -------- | ----------------------------------------------- |
| Basic.DataPath | String  | 规则配置文件路径   | Y    | -        | 类型为 [FilePath](../00-common.md#3-文件路径filepath) |
| Log.OpenDebug  | Boolean | 是否开启 debug 模式 | N    | 默认值 `False` | -                                               |

## 配置示例

```ini
[Basic]
DataPath = ../data/mod_tcp_keepalive/tcp_keepalive.data

[Log]
OpenDebug = false
```
