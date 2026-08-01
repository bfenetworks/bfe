# mod_markdown 基础配置

## 配置简介

`mod_markdown.conf` 是 `mod_markdown` 模块的基础配置文件，用于指定 Markdown 规则文件路径等。

## 配置描述

| 配置项                | 类型    | 参数含义              | 必填 | 补充描述                                   | 合法性条件                                                   |
| --------------------- | ------- | --------------------- | ---- | ------------------------------------------ | ------------------------------------------------------------ |
| Basic.ProductRulePath | String  | Markdown 规则文件路径 | Y    | 默认值为 `mod_markdown/markdown_rule.data` | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Log.OpenDebug         | Boolean | 是否开启 debug 日志   | N    | 默认值为 `False`                           | -                                                            |

## 配置示例

```ini
[Basic]
ProductRulePath = mod_markdown.data

[Log]
OpenDebug = true
```
