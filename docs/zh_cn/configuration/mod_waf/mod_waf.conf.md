# mod_waf 基础配置

## 配置简介

`mod_waf.conf` 是 `mod_waf` 模块的基础配置文件，用于指定 WAF 规则文件路径及日志输出配置。

## 配置描述

| 配置项                | 类型    | 参数含义                                                   | 必填 | 补充描述                                                     | 合法性条件                                                   |
| --------------------- | ------- | ---------------------------------------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Basic.ProductRulePath | String  | WAF 规则文件路径                                           | Y    | 默认值为 `mod_waf/waf_rule.data`                             | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Log.LogFile           | String  | 日志文件路径，用来将日志输出到单个文件中（不进行日志切割） | N    | 类型为 [FilePath](../00-common.md#3-文件路径filepath)           | 与 `LogPrefix`、`LogDir`、`RotateWhen`、`BackupCount` 互斥，不可同时配置 |
| Log.LogPrefix         | String  | 日志文件前缀                                               | N    | 通常与 `LogDir` 配合使用                                     | `LogFile` 未配置时必填                                       |
| Log.LogDir            | String  | 日志文件目录                                               | N    | 类型为 [DirPath](../00-common.md#4-目录路径dirpath)             | `LogFile` 未配置时必填                                       |
| Log.RotateWhen        | String  | 日志切割时间                                               | N    | 支持 `M` / `H` / `D` / `MIDNIGHT` / `NEXTHOUR`               | `LogFile` 未配置时必填；取值须为 `M`、`H`、`D`、`MIDNIGHT`、`NEXTHOUR` 之一 |
| Log.BackupCount       | Integer | 最大的日志存储数量                                         | N    | 日志轮转后保留的最大文件数                                   | `LogFile` 未配置时必填；须大于 0                             |

## 配置示例

```ini
[Basic]
ProductRulePath = mod_waf/waf_rule.data

[Log]
LogPrefix = waf
LogDir = ../log
RotateWhen = NEXTHOUR
BackupCount = 24
```
