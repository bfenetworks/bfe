# mod_prison 基础配置

## 配置简介

`mod_prison.conf` 是 `mod_prison` 模块的基础配置文件，用于指定规则配置文件路径等。

## 配置描述

| 配置项                | 类型    | 参数含义           | 必填 | 补充描述                            | 合法性条件                                                   |
| --------------------- | ------- | ------------------ | ---- | ----------------------------------- | ------------------------------------------------------------ |
| Basic.ProductRulePath | String  | 规则配置文件路径   | Y    | 默认值为 `mod_prison/prison.data`   | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Log.OpenDebug         | Boolean | 是否开启 debug 日志 | N    | 默认值为 `False`                    | -                                                            |

## 配置示例

```ini
[Basic]
ProductRulePath = mod_prison/prison.data

[Log]
OpenDebug = false
```
