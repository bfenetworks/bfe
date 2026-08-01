# mod_static 基础配置

## 配置简介

`mod_static.conf` 是 `mod_static` 模块的基础配置文件，用于指定静态文件规则配置文件、MIME 配置文件路径以及压缩开关等。

## 配置描述

| 配置项               | 类型    | 参数含义             | 必填 | 补充描述                               | 合法性条件                                                   |
| -------------------- | ------- | -------------------- | ---- | -------------------------------------- | ------------------------------------------------------------ |
| Basic.DataPath       | String  | 规则配置文件路径     | Y    | 默认值为 `mod_static/static_rule.data` | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Basic.MimeTypePath   | String  | MIME 配置文件路径    | Y    | 默认值为 `mod_static/mime_type.data`   | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Basic.EnableCompress | Boolean | 是否启用静态文件压缩 | N    | 默认值为 `False`                       | -                                                            |
| Log.OpenDebug        | Boolean | 是否开启 debug 日志  | N    | 默认值为 `False`                       | -                                                            |

## 配置示例

```ini
[Basic]
DataPath = mod_static/static_rule.data
MimeTypePath = mod_static/mime_type.data
EnableCompress = false

[Log]
OpenDebug = false
```
