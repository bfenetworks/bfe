# mod_tag 基础配置

## 配置简介

`mod_tag.conf` 是 `mod_tag` 模块的基础配置文件，用于指定规则配置文件路径等。

## 配置描述

| 配置项         | 类型    | 参数含义                 | 必填 | 补充描述                          | 合法性条件                                                   |
| -------------- | ------- | ------------------------ | ---- | --------------------------------- | ------------------------------------------------------------ |
| Basic.DataPath | String  | 规则配置文件路径         | Y    | 默认值为 `mod_tag/tag_rule.data`  | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Log.OpenDebug  | Boolean | 是否启用模块调试日志开关 | N    | 默认值 `False`                    | -                                                            |

## 配置示例

```ini
[Basic]
DataPath = mod_tag/tag_rule.data

[Log]
OpenDebug = false
```
