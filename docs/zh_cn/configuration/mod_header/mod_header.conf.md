# mod_header 基础配置

## 配置简介

`mod_header.conf` 是 `mod_header` 模块的基础配置文件，用于指定规则配置文件路径等。

## 配置描述

| 配置项                     | 类型    | 参数含义                 | 必填 | 补充描述                              | 合法性条件                                                   |
| -------------------------- | ------- | ------------------------ | ---- | ------------------------------------- | ------------------------------------------------------------ |
| Basic.DataPath             | String  | 规则配置文件路径         | Y    | 默认值为 `mod_header/mod_header.data` | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Basic.DisableDefaultHeader | Boolean | 是否禁用默认 Header      | N    | 默认值为 `False`                      | -                                                            |
| Log.OpenDebug              | Boolean | 是否开启 debug 日志      | N    | 默认值为 `False`                      | -                                                            |

## 配置示例

```ini
[Basic]
DataPath = mod_header/mod_header.data
DisableDefaultHeader = false

[Log]
OpenDebug = false
```
