# mod_ai_rate_limit Rule Configuration

## Introduction

`ai_rate_limit.data` is the rule configuration file of the `mod_ai_rate_limit` module, used to configure AI request rate limit rules, rate limit policies, and the bindings from apikey to policy IDs.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | - | Type is [Version](../00-common.md#5-version) |
| Config | Object | Rate limit rules for each product | Y | - | Must be a non-empty object |
| Config{k} | String | Product name | Y | Key of `Config` | Non-empty string |
| Config{v} | Array | List of rate limit rules under the product | Y | - | Array of Object |
| Config{v}[].cond | String | Condition to match the request | Y | Syntax see [Condition](../../condition/condition_grammar.md) | Non-empty string, must be a valid condition expression |
| Config{v}[].hit_action | Object | Action when rate limit is hit | Y | - | Must be a non-empty object |
| Config{v}[].hit_action.cmd | String | Action command | Y | E.g. `PASS`, `FINISH` | Non-empty string |
| Config{v}[].hit_action.params | Array | Action parameters | N | - | Array of String |
| RateLimitPolicies | Object | Collection of rate limit policies | Y | - | Must be a non-empty object |
| RateLimitPolicies{k} | String | Policy ID | Y | Key of `RateLimitPolicies` | Non-empty string |
| RateLimitPolicies{v} | Object | Detailed policy information | Y | - | Must be a non-empty object |
| RateLimitPolicies{v}.name | String | Policy name | Y | - | Non-empty string |
| RateLimitPolicies{v}.enabled | Boolean | Whether the policy is enabled | Y | - | - |
| RateLimitPolicies{v}.models | Array | List of models the policy applies to | N | Element type is model name string | Array of String |
| RateLimitPolicies{v}.rules | Object | Rate limit rule details | Y | - | Must be a non-empty object |
| RateLimitPolicies{v}.rules.tpm | Array | List of TPM rate limit rules | N | - | Array of Object |
| RateLimitPolicies{v}.rules.tpm[].name | String | Rule name | Y | - | Non-empty string |
| RateLimitPolicies{v}.rules.tpm[].window_minutes | Integer | Time window | Y | Unit: minutes | Positive integer |
| RateLimitPolicies{v}.rules.tpm[].max_tokens | Integer | Max token count within the window | Y | - | Positive integer |
| RateLimitPolicies{v}.rules.tpm[].step_minutes | Integer | Step size | Y | Unit: minutes | Positive integer |
| RateLimitPolicies{v}.rules.tpm[].models | Array | List of models the rule applies to | N | Applies to all models if not specified | Array of String |
| RateLimitPolicies{v}.rules.rpm | Array | List of RPM rate limit rules | N | - | Array of Object |
| RateLimitPolicies{v}.rules.rpm[].name | String | Rule name | Y | - | Non-empty string |
| RateLimitPolicies{v}.rules.rpm[].window_minutes | Integer | Time window | Y | Unit: minutes | Positive integer |
| RateLimitPolicies{v}.rules.rpm[].max_requests | Integer | Max request count within the window | Y | - | Positive integer |
| RateLimitPolicies{v}.rules.rpm[].burst | Integer | Burst request count | Y | - | Non-negative integer |
| RateLimitPolicies{v}.rules.rpm[].models | Array | List of models the rule applies to | N | Applies to all models if not specified | Array of String |
| RateLimitPolicies{v}.rules.max_concurrency | Integer | Max concurrency | N | - | Non-negative integer |
| ApikeyRateLimitPolicyBindings | Object | Binding from apikey to list of policy IDs | N | - | Key-value object |
| ApikeyRateLimitPolicyBindings{k} | String | apikey | Y | Key of the binding | Non-empty string |
| ApikeyRateLimitPolicyBindings{v} | Array | List of policy IDs bound to the apikey | Y | - | Non-empty array of String |

## Configuration Example

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
