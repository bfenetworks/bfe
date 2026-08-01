# mod_waf 规则配置

## 配置简介

`waf_rule.data` 是 `mod_waf` 模块的规则配置文件，用于配置各产品线下的 WAF 检测规则。

## 配置描述

| 配置项                 | 类型     | 参数含义                     | 必填 | 补充描述                                                   | 合法性条件                                           |
| ---------------------- | -------- | ---------------------------- | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version                | String   | 配置文件版本                 | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                 | Object   | 各产品线的 WAF 规则          | Y    | 以产品线名称为键                                           | -                                                    |
| Config{k}              | String   | 产品线名称                   | Y    | -                                                          | -                                                    |
| Config{v}              | Array    | 产品线下的 WAF 规则列表      | Y    | -                                                          | -                                                    |
| Config{v}[]            | Object   | 一条 WAF 规则                | Y    | -                                                          | -                                                    |
| Config{v}[].Cond       | String   | 匹配请求的条件               | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config{v}[].BlockRules | []String | 命中后直接拦截的规则名列表   | N    | 至少配置 `BlockRules` 或 `CheckRules` 中的一个             | 规则名须为当前模块支持的 WAF 规则                    |
| Config{v}[].CheckRules | []String | 命中后仅记录日志的规则名列表 | N    | 至少配置 `BlockRules` 或 `CheckRules` 中的一个             | 规则名须为当前模块支持的 WAF 规则                    |

## 支持的 WAF 规则

| 规则名      | 含义             |
| ----------- | ---------------- |
| RuleBashCmd | bash 命令注入检测 |

## 配置示例

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
