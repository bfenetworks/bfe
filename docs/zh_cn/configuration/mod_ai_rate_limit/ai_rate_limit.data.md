# mod_ai_rate_limit 规则配置

## 配置简介

`ai_rate_limit.data` 用于配置各产品线的 AI 请求限流规则、限流策略以及 apikey 到策略的绑定关系。

## 配置描述

| 配置项                                           | 类型    | 参数含义                                | 必填 | 补充描述 | 合法性条件 |
| ------------------------------------------------ | ------- | --------------------------------------- | ---- | -------- | ---------- |
| Version                                          | String  | 配置文件版本                            | Y    | -        | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                                           | Object  | 各产品线的限流规则                      | Y    | -        | 不可为空对象 |
| Config{k}                                        | String  | 产品线名称                              | Y    | 作为 Config 的键 | 非空字符串 |
| Config{v}                                        | Array   | 该产品线下的限流规则列表                | Y    | -        | 数组，元素为 Object |
| Config{v}[].cond                                 | String  | 匹配请求的条件                          | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | 非空字符串，须为合法条件表达式 |
| Config{v}[].hit_action                           | Object  | 命中限流后的动作                        | Y    | -        | 不可为空对象 |
| Config{v}[].hit_action.cmd                       | String  | 动作指令                                | Y    | 例如 PASS、FINISH | 非空字符串 |
| Config{v}[].hit_action.params                    | Array   | 动作参数                                | N    | -        | 字符串数组 |
| RateLimitPolicies                                | Object  | 限流策略集合                            | Y    | -        | 不可为空对象 |
| RateLimitPolicies{k}                             | String  | 策略 ID                                 | Y    | 作为 RateLimitPolicies 的键 | 非空字符串 |
| RateLimitPolicies{v}                             | Object  | 策略详细信息                            | Y    | -        | 不可为空对象 |
| RateLimitPolicies{v}.name                        | String  | 策略名称                                | Y    | -        | 非空字符串 |
| RateLimitPolicies{v}.enabled                     | Boolean | 是否启用                                | Y    | -        | - |
| RateLimitPolicies{v}.models                      | Array   | 该策略适用的模型列表                    | N    | 元素为模型名称字符串 | 字符串数组 |
| RateLimitPolicies{v}.rules                       | Object  | 限流规则详情                            | Y    | -        | 不可为空对象 |
| RateLimitPolicies{v}.rules.tpm                   | Array   | TPM（每分钟 Token 数）限流规则列表      | N    | -        | 元素为 Object |
| RateLimitPolicies{v}.rules.tpm[].name            | String  | 规则名称                                | Y    | -        | 非空字符串 |
| RateLimitPolicies{v}.rules.tpm[].window_minutes  | Integer | 时间窗口                                | Y    | 单位：分钟 | 正整数 |
| RateLimitPolicies{v}.rules.tpm[].max_tokens      | Integer | 窗口内最大 Token 数                     | Y    | -        | 正整数 |
| RateLimitPolicies{v}.rules.tpm[].step_minutes    | Integer | 步长                                    | Y    | 单位：分钟 | 正整数 |
| RateLimitPolicies{v}.rules.tpm[].models          | Array   | 该规则适用的模型列表                    | N    | 未指定时适用于所有模型 | 字符串数组 |
| RateLimitPolicies{v}.rules.rpm                   | Array   | RPM（每分钟请求数）限流规则列表         | N    | -        | 元素为 Object |
| RateLimitPolicies{v}.rules.rpm[].name            | String  | 规则名称                                | Y    | -        | 非空字符串 |
| RateLimitPolicies{v}.rules.rpm[].window_minutes  | Integer | 时间窗口                                | Y    | 单位：分钟 | 正整数 |
| RateLimitPolicies{v}.rules.rpm[].max_requests    | Integer | 窗口内最大请求数                        | Y    | -        | 正整数 |
| RateLimitPolicies{v}.rules.rpm[].burst           | Integer | 突发请求数                              | Y    | -        | 非负整数 |
| RateLimitPolicies{v}.rules.rpm[].models          | Array   | 该规则适用的模型列表                    | N    | 未指定时适用于所有模型 | 字符串数组 |
| RateLimitPolicies{v}.rules.max_concurrency       | Integer | 最大并发数                              | N    | -        | 非负整数 |
| ApikeyRateLimitPolicyBindings                    | Object  | apikey 到策略 ID 列表的绑定关系         | N    | -        | 键值对对象 |
| ApikeyRateLimitPolicyBindings{k}                 | String  | apikey                                  | Y    | 作为绑定关系的键 | 非空字符串 |
| ApikeyRateLimitPolicyBindings{v}                 | Array   | 该 apikey 绑定的策略 ID 列表            | Y    | -        | 非空字符串数组 |

## 配置示例

```json
{
    "Version": "1.0",
    "Config": {
        "AI_product": [
            {
                "cond": "default_t()",
                "hit_action": {
                    "cmd": "PASS"
                }
            }
        ]
    },
    "RateLimitPolicies": {
        "rlp-0001": {
            "name": "ratelimitX",
            "enabled": true,
            "rules": {
                "tpm": [
                    {"name":"abc0", "window_minutes": 1, "max_tokens": 10000, "step_minutes": 1, "models": ["gpt-4", "gpt-4o"]},
                    {"name":"abc1", "window_minutes": 10, "max_tokens": 50000, "step_minutes": 1, "models": ["gpt-3.5"]},
                    {"name":"abc2", "window_minutes": 60, "max_tokens": 200000, "step_minutes": 5}
                ],
                "rpm": [
                    {"name":"abc0", "window_minutes": 1, "max_requests": 100, "burst":1, "models": ["gpt-4"]}
                ],
                "max_concurrency": 50
            }
        }
    },
    "ApikeyRateLimitPolicyBindings": {
        "ak-2v8x9k3m7p": ["rlp-0001"]
    }
}
```
