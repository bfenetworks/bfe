# mod_errors 基础配置

## 配置简介

`mod_errors.conf` 是 `mod_errors` 模块的基础配置文件，用于指定错误响应规则数据文件路径及日志选项。

## 配置描述

| 配置项          | 类型    | 参数含义                 | 必填 | 补充描述                                                     | 合法性条件                                                   |
| --------------- | ------- | ------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Basic.DataPath  | String  | 规则配置的文件路径       | N    | 默认值为 `mod_errors/mod_errors.data`                        | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；空时使用默认值 |
| Log.OpenDebug   | Boolean | 是否开启 debug 日志      | N    | 默认值 `False`                                               | -                                                            |

## 配置示例

```ini
[Basic]
DataPath = mod_errors/errors_rule.data

[Log]
OpenDebug = false
```
