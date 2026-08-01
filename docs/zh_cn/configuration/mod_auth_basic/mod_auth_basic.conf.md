# mod_auth_basic 基础配置

## 配置简介

`mod_auth_basic.conf` 是 `mod_auth_basic` 模块的基础配置文件，用于指定规则配置文件路径等。

## 配置描述

| 配置项         | 类型    | 参数含义             | 必填 | 补充描述      | 合法性条件                                                   |
| -------------- | ------- | -------------------- | ---- | ------------- | ------------------------------------------------------------ |
| Basic.DataPath | String  | 规则配置文件的文件路径 | Y    | -             | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Log.OpenDebug  | Boolean | 是否开启 debug 日志  | N    | 默认值`False` | -                                                            |

## 配置示例

```ini
[Basic]
DataPath = mod_auth_basic/auth_basic_rule.data

[Log]
OpenDebug = false
```
