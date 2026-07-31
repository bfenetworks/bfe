# mod_waf

## 模块简介

mod_waf 基于自定义规则，对请求执行 WAF（Web 应用防火墙）检测。支持 block 规则（命中后直接拦截）和 check 规则（命中后记录日志）。

## 基础配置

### 配置描述

模块配置文件: conf/mod_waf/mod_waf.conf

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Basic.ProductRulePath | String<br>WAF 规则文件路径<br>默认值 mod_waf/waf_rule.data |
| Log.LogFile           | String<br>日志文件路径，用来将日志输出到单个文件中（不进行日志切割） |
| Log.LogPrefix         | String<br>日志文件前缀 |
| Log.LogDir            | String<br>日志文件目录 |
| Log.RotateWhen        | String<br>日志切割时间，支持 M/H/D/MIDNIGHT/NEXTHOUR |
| Log.BackupCount       | Integer<br>最大的日志存储数量 |

### 配置示例

```ini
[Basic]
ProductRulePath = mod_waf/waf_rule.data

[Log]
LogPrefix = waf
LogDir = ../log
RotateWhen = NEXTHOUR
BackupCount = 24
```

## 规则配置

### 配置描述

规则配置文件: waf_rule.data

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config | Object<br>各产品线的 WAF 规则 |
| Config{k} | String<br>产品线名称 |
| Config{v} | Object<br>产品线下的 WAF 规则列表 |
| Config{v}[] | Object<br>WAF 规则详细信息 |
| Config{v}[].Cond | String<br>描述匹配请求的条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].BlockRules | []String<br>命中后直接拦截的规则名列表 |
| Config{v}[].CheckRules | []String<br>命中后仅记录日志的规则名列表 |

### 支持的 WAF 规则

| 规则名       | 含义         |
| ------------ | ------------ |
| RuleBashCmd  | bash 命令注入检测 |

### 配置示例

```json
{
    "Version": "2019-12-10184356",
    "Config": {
        "example_product": [
            {
                "Cond": "default_t()",
                "BlockRules": [
                    "RuleBashCmd"
                ]
            }
        ]
    }
}
```

## 监控项

| 监控项           | 描述                     |
| ---------------- | ------------------------ |
| CHECKED_REQ      | WAF 检测的请求数         |
| HIT_BLOCKED_REQ  | 命中拦截规则的请求数     |
| HIT_CHECKED_RULE | 命中检测规则的请求数     |
| BLOCKED_RULE_ERROR | 拦截规则执行错误次数     |
| CHECKED_RULE_ERROR | 检测规则执行错误次数     |
