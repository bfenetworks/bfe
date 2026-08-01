# mod_ai_token_auth 规则配置

## 配置简介

`token_rule.data` 是 `mod_ai_token_auth` 模块的规则配置文件，用于声明 api-key、配额计划以及按产品线的鉴权规则。

## 配置描述

| 配置项                         | 类型    | 参数含义                         | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ------------------------------ | ------- | -------------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version                        | String  | 配置文件版本                     | Y    | 通常采用时间戳格式，如 `20190101000000`                      | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| QuotaPlans                     | Object  | 所有产品线的配额计划声明         | Y    | 以产品线名称为键                                             | -                                                            |
| QuotaPlans{k}                  | String  | 产品线名称                       | Y    | -                                                            | -                                                            |
| QuotaPlans{v}                  | Array   | 产品线下的配额计划列表           | Y    | -                                                            | -                                                            |
| QuotaPlans{v}[]                | Object  | 配额计划                         | Y    | -                                                            | -                                                            |
| QuotaPlans{v}[].id             | String  | 配额计划 ID                      | Y    | -                                                            | -                                                            |
| QuotaPlans{v}[].unlimited      | Boolean | 是否无限配额                     | Y    | -                                                            | -                                                            |
| QuotaPlans{v}[].pass_no_quota  | Boolean | 配额不足时是否放行               | Y    | -                                                            | -                                                            |
| QuotaPlans{v}[].redis_key      | String  | Redis 中存储配额的 key           | N    | unlimited 为 true 时可不配置                                 | -                                                            |
| QuotaPlans{v}[].create_time    | Integer | 创建时间（Unix Time）            | N    | -                                                            | -                                                            |
| QuotaPlans{v}[].expired_time   | Integer | 过期时间（Unix Time）            | N    | `-1` 表示永不过期                                            | 必须大于等于 `-1`                                            |
| QuotaPlans{v}[].quota          | Integer | 配额总量（单位：token）          | N    | unlimited 为 false 时必填                                    | unlimited 为 false 时必须大于 0                              |
| QuotaPlans{v}[].reset_mode     | Integer | 重置模式                         | Y    | `0` - 非周期性；`1` - 周期性的配额包                         | 取值范围为 `0`、`1`                                          |
| Tokens                         | Object  | 所有产品线的 api-key 声明        | Y    | 以产品线名称为键                                             | -                                                            |
| Tokens{k}                      | String  | 产品线名称                       | Y    | -                                                            | -                                                            |
| Tokens{v}                      | Object  | 该产品线下的所有 api-key         | Y    | -                                                            | -                                                            |
| Tokens{v}{k}                   | String  | api-key                          | Y    | -                                                            | -                                                            |
| Tokens{v}{v}                   | Object  | 一个 api-key 声明                | Y    | -                                                            | -                                                            |
| Tokens{v}{v}.key               | String  | api-key                          | Y    | 须与外层键一致                                               | -                                                            |
| Tokens{v}{v}.enabled           | Integer | 是否启用                         | N    | -                                                            | -                                                            |
| Tokens{v}{v}.status            | Integer | api-key 状态                     | Y    | `1` - Enabled；`2` - Disabled；`3` - Expired；`4` - Exhausted | 取值范围为 `1`、`2`、`3`、`4`                                |
| Tokens{v}{v}.name              | String  | 名称                             | N    | -                                                            | -                                                            |
| Tokens{v}{v}.update_time       | Integer | 更新时间（Unix Time）            | N    | 改变意味着开启一个新的配额消费周期                           | -                                                            |
| Tokens{v}{v}.expired_time      | Integer | 过期时间（Unix Time）            | N    | `-1` 表示永不过期                                            | 必须大于等于 `-1`                                            |
| Tokens{v}{v}.unlimited_quota   | Boolean | 是否无限配额                     | Y    | -                                                            | -                                                            |
| Tokens{v}{v}.allow_models      | String  | 允许的模型列表                   | N    | 多个模型名以逗号分隔                                         | 不能包含空字符串                                             |
| Tokens{v}{v}.block_models      | String  | 禁止的模型列表                   | N    | 多个模型名以逗号分隔                                         | 不能包含空字符串                                             |
| Tokens{v}{v}.subnet            | String  | 允许的源 IP 子网                 | N    | 多个子网以逗号分隔                                           | 须为有效的 CIDR 格式                                         |
| Tokens{v}{v}.tags              | Array   | api-key 标签列表                 | N    | -                                                            | -                                                            |
| Tokens{v}{v}.tags[]            | Object  | api-key 标签                     | N    | -                                                            | -                                                            |
| Tokens{v}{v}.tags[].key        | String  | 标签名                           | N    | 如 `department`                                              | -                                                            |
| Tokens{v}{v}.tags[].value      | String  | 标签值                           | N    | 如 `engineering`                                             | -                                                            |
| Tokens{v}{v}.quota_plans       | Array   | 关联的配额计划 ID 列表           | N    | unlimited_quota 为 false 时必填                              | unlimited_quota 为 false 时须非空                            |
| Config                         | Object  | 所有产品线的 api-key 鉴权规则配置 | Y    | 以产品线名称为键                                             | -                                                            |
| Config{k}                      | String  | 产品线名称                       | Y    | -                                                            | -                                                            |
| Config{v}                      | Array   | 产品线下 api-key 鉴权规则列表    | Y    | -                                                            | -                                                            |
| Config{v}[]                    | Object  | api-key 鉴权规则                 | Y    | -                                                            | -                                                            |
| Config{v}[].cond               | String  | 匹配条件                         | Y    | 语法详见 [Condition](../../condition/condition_grammar.md)   | -                                                            |
| Config{v}[].action             | Object  | 动作                             | Y    | 只支持 `{ "cmd": "CHECK_TOKEN" }`                          | -                                                            |
| Config{v}[].action.cmd         | String  | 动作命令                         | Y    | 固定为 `CHECK_TOKEN`                                         | 取值范围为 `CHECK_TOKEN`                                     |

## 配置示例

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
                "reset_mode": 1
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
