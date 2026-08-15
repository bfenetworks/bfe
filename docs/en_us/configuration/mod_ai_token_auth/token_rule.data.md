# mod_ai_token_auth Rule Configuration

## Configuration Introduction

`token_rule.data` is the rule configuration file for the `mod_ai_token_auth` module, used to declare API-keys, quota plans, and API-key authentication rules for each product line.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `20190101000000` | Type is [Version](../00-common.md#5-version) |
| QuotaPlans | Object | Quota plan declarations for all product lines | Y | Key is product line name | - |
| QuotaPlans{k} | String | Product line name | Y | - | - |
| QuotaPlans{v} | Array | Quota plan list under a product line | Y | - | - |
| QuotaPlans{v}[] | Object | Quota plan | Y | - | - |
| QuotaPlans{v}[].id | String | Quota plan ID | Y | - | - |
| QuotaPlans{v}[].unlimited | Boolean | Unlimited quota or not | Y | - | - |
| QuotaPlans{v}[].pass_no_quota | Boolean | Allow request when quota is insufficient | Y | - | - |
| QuotaPlans{v}[].redis_key | String | Redis key for storing quota | N | Optional when `unlimited` is true | - |
| QuotaPlans{v}[].create_time | Integer | Create time (Unix Time) | N | - | - |
| QuotaPlans{v}[].expired_time | Integer | Expiry time (Unix Time) | N | `-1` means never expires | Must be greater than or equal to `-1` |
| QuotaPlans{v}[].quota | Integer | Total quota | N | Unit is determined by the `unit` field; when `unit=RMB`, this is a fixed-point integer with precision `1e-8` yuan; required when `unlimited` is false | Must be greater than 0 when `unit=total_token` and `unlimited` is false; must be greater than or equal to 0 when `unit=RMB` and `unlimited` is false |
| QuotaPlans{v}[].reset_mode | Integer | Reset mode | Y | `0` - non-periodic; `1` - periodic quota package | Value must be `0` or `1` |
| QuotaPlans{v}[].unit | String | Quota unit | N | Defaults to `total_token` | Value must be `total_token` or `RMB` |
| Tokens | Object | API-key declarations for all product lines | Y | Key is product line name | - |
| Tokens{k} | String | Product line name | Y | - | - |
| Tokens{v} | Object | All API-keys under a product line | Y | - | - |
| Tokens{v}{k} | String | An API-key | Y | - | - |
| Tokens{v}{v} | Object | An API-key declaration | Y | - | - |
| Tokens{v}{v}.key | String | API-key | Y | Must be consistent with the outer key | - |
| Tokens{v}{v}.enabled | Integer | Whether enabled | N | - | - |
| Tokens{v}{v}.status | Integer | API-key status | Y | `1` - Enabled; `2` - Disabled; `3` - Expired; `4` - Exhausted | Value must be `1`, `2`, `3`, or `4` |
| Tokens{v}{v}.name | String | Name | N | - | - |
| Tokens{v}{v}.update_time | Integer | Update time (Unix Time) | N | Change means a new quota consumption cycle starts, recalculating used quota | - |
| Tokens{v}{v}.expired_time | Integer | Expiry time (Unix Time) | N | `-1` means never expires | Must be greater than or equal to `-1` |
| Tokens{v}{v}.unlimited_quota | Boolean | Unlimited quota or not | Y | - | - |
| Tokens{v}{v}.allow_models | String | Allowed model list | N | Multiple model names separated by commas | Cannot contain empty strings |
| Tokens{v}{v}.block_models | String | Blocked model list | N | Multiple model names separated by commas | Cannot contain empty strings |
| Tokens{v}{v}.subnet | String | Allowed source IP subnet | N | Multiple subnets separated by commas | Must be valid CIDR format |
| Tokens{v}{v}.tags | Array | API-key tag list | N | - | - |
| Tokens{v}{v}.tags[] | Object | API-key tag | N | - | - |
| Tokens{v}{v}.tags[].key | String | Tag name | N | E.g., `department` | - |
| Tokens{v}{v}.tags[].value | String | Tag value | N | E.g., `engineering` | - |
| Tokens{v}{v}.quota_plans | Array | Associated quota plan ID list | N | Required when `unlimited_quota` is false | Must be non-empty when `unlimited_quota` is false |
| Config | Object | API-key authentication rule configuration for all product lines | Y | Key is product line name | - |
| Config{k} | String | Product line name | Y | - | - |
| Config{v} | Array | API-key authentication rule list under a product line | Y | - | - |
| Config{v}[] | Object | API-key authentication rule | Y | - | - |
| Config{v}[].cond | String | Matching condition | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| Config{v}[].action | Object | Action | Y | Only one action is supported: `{ "cmd": "CHECK_TOKEN" }` | - |
| Config{v}[].action.cmd | String | Action command | Y | Fixed as `CHECK_TOKEN` | Value must be `CHECK_TOKEN` |

## Configuration Example

```json
{
    "Version": "20190101000000",
    "QuotaPlans": {
        "example_product": [
            {
                "id": "daily_quota",
                "unlimited": false,
                "pass_no_quota": false,
                "redis_key": "ai:quota:daily_quota",
                "create_time": 1672531200,
                "expired_time": -1,
                "quota": 100000,
                "reset_mode": 1,
                "unit": "total_token"
            },
            {
                "id": "daily_rmb_quota",
                "unlimited": false,
                "pass_no_quota": false,
                "redis_key": "ai:quota:daily_rmb_quota",
                "create_time": 1672531200,
                "expired_time": -1,
                "quota": 90000000,
                "reset_mode": 0,
                "unit": "RMB"
            }
        ]
    },
    "Tokens": {
        "example_product": {
            "TESTKEY": {
                "key": "TESTKEY",
                "status": 1,
                "name": "test",
                "expired_time": -1,
                "unlimited_quota": false,
                "allow_models": "model_a,model_b",
                "block_models": "model_c",
                "subnet": "192.168.0.0/24",
                "tags": [
                    {"key": "department", "value": "engineering"}
                ],
                "quota_plans": ["daily_quota"]
            }
        }
    },
    "Config": {
        "example_product": [
            {
                "cond": "default_t()",
                "action": {
                    "cmd": "CHECK_TOKEN"
                }
            }
        ]
    }
}
```

> Note:
> - When `unit = total_token`, `quota` is an integer number of tokens.
> - When `unit = RMB`, `quota` is a fixed-point integer with precision `1e-8` yuan (i.e., 1 unit = 0.00000001 yuan). For example, `90000000` means `0.9` yuan.
