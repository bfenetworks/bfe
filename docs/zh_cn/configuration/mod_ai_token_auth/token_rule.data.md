# mod_ai_token_auth 规则配置

## 1. 配置简介

`token_rule.data` 是 `mod_ai_token_auth` 模块的规则配置文件，用于声明 api-key、配额计划以及按产品线的鉴权规则。

## 2. 顶层结构

```json
{
    "Version": "20190101000000",
    "QuotaPlans": { /* 按产品线分组的配额计划定义 */ },
    "Tokens": { /* 按产品线分组的 api-key 声明 */ },
    "Config": { /* 按产品线分组的 api-key 鉴权规则 */ }
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| Version | string | Y | 配置文件版本，通常采用时间戳格式，如 `20190101000000` |
| QuotaPlans | object | Y | 按产品线分组的配额计划声明 |
| Tokens | object | Y | 按产品线分组的 api-key 声明 |
| Config | object | Y | 按产品线分组的 api-key 鉴权规则配置 |

## 3. QuotaPlans 结构（配额计划定义）

配额计划在顶层 `QuotaPlans` 中按产品线分组，Token 通过 `quota_plans` 数组引用这些计划的 ID。

```json
{
    "QuotaPlans": {
        "example_product": [
            {
                "Id": "daily_quota",
                "Unlimited": false,
                "PassNoQuota": false,
                "RedisKey": "ai:quota:daily_quota",
                "ExpiredTime": -1,
                "Quota": 100000,
                "Unit": "total_token"
            }
        ]
    }
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| Id | string | Y | 配额计划 ID | 非空字符串 |
| Unlimited | bool | Y | 是否无限配额 | - |
| PassNoQuota | bool | Y | 配额不足时是否放行；为 `true` 时跳过该计划的余额检查 | - |
| RedisKey | string | N | Redis 中存储配额余额的 Key | `Unlimited` 为 `false` 时必须有有效值，否则运行时扣减/校验余额会失败 |
| ExpiredTime | int64 | N | 过期时间；`-1` 表示永不过期 | 必须大于等于 `-1` |
| Quota | int64 | N | 配额总量 | `Unit=total_token` 且 `Unlimited=false` 时必须大于 0；`Unit=RMB` 且 `Unlimited=false` 时必须大于等于 0 |
| Unit | string | N | 配额单位 | `total_token` 或 `RMB`；为空时默认 `total_token` |

## 4. Tokens 结构（api-key 声明）

api-key 在顶层 `Tokens` 中按产品线分组，外层 key 为 api-key 值。

```json
{
    "Tokens": {
        "example_product": {
            "TESTKEY": {
                "key": "TESTKEY",
                "key_id": "test_key_id",
                "enabled": true,
                "expired_time": -1,
                "unlimited_quota": false,
                "allow_models": "model_a,model_b",
                "block_models": "model_c",
                "subnet": "192.168.0.0/24",
                "Tags": [
                    {"TagName": "department", "TagValue": "engineering", "TagLevel": 3}
                ],
                "quota_plans": ["daily_quota"]
            }
        }
    }
}
```

| 字段 | 类型 | 必填 | 说明 | 合法性条件 |
|------|------|------|------|------------|
| key | string | Y | api-key 值 | - |
| key_id | string | Y | api-key 标识 ID | 非空字符串 |
| enabled | bool | Y | 是否启用 | `true` 启用，`false` 禁用 |
| expired_time | int64 | N | 过期时间；`-1` 表示永不过期 | 必须大于等于 `-1` |
| unlimited_quota | bool | Y | 是否无限配额 | - |
| allow_models | string | N | 允许的模型列表 | 多个模型名以逗号分隔；不能包含空字符串；空或 `""` 表示不限制 |
| block_models | string | N | 禁止的模型列表 | 多个模型名以逗号分隔；不能包含空字符串；空或 `""` 表示无禁止模型 |
| subnet | string | N | 允许的源 IP 子网 | 多个子网以逗号分隔；须为有效的 CIDR 格式；空或 `""` 表示不限制 |
| Tags | []ApikeyTag | N | api-key 标签列表 | JSON 字段名为 `Tags`；因结构体未设置 json tag，标签项字段名为 `TagName`/`TagValue`（大小写不敏感） |
| quota_plans | []string | N | 关联的配额计划 ID 列表 | `unlimited_quota` 为 `false` 时必填非空；引用的 ID 必须在同产品线的 `QuotaPlans` 中已定义 |

### 4.1 ApikeyTag 结构

| 字段 | 类型 | 必填 | 说明 | 示例 |
|------|------|------|------|------|
| TagName | string | N | 标签名 | `department` |
| TagValue | string | N | 标签值 | `engineering` |
| TagLevel | int | N | 标签级别，取值为 1~5 的整数 | `3` |

## 5. Config 结构（api-key 鉴权规则）

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "default_t()",
                "Action": {
                    "Cmd": "CHECK_TOKEN"
                }
            }
        ]
    }
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| Cond | string | Y | 路由匹配条件表达式，语法详见 [Condition](../../condition/condition_grammar.md) |
| Action | object | Y | 匹配后执行的动作 |
| Action.Cmd | string | Y | 动作命令，固定为 `CHECK_TOKEN` |

## 6. 完整配置示例

```json
{
    "Version": "20190101000000",
    "QuotaPlans": {
        "example_product": [
            {
                "Id": "daily_quota",
                "Unlimited": false,
                "PassNoQuota": false,
                "RedisKey": "ai:quota:daily_quota",
                "ExpiredTime": -1,
                "Quota": 100000,
                "Unit": "total_token"
            },
            {
                "Id": "daily_rmb_quota",
                "Unlimited": false,
                "PassNoQuota": false,
                "RedisKey": "ai:quota:daily_rmb_quota",
                "ExpiredTime": -1,
                "Quota": 90000000,
                "Unit": "RMB"
            }
        ]
    },
    "Tokens": {
        "example_product": {
            "TESTKEY": {
                "key": "TESTKEY",
                "key_id": "test_key_id",
                "enabled": true,
                "expired_time": -1,
                "unlimited_quota": false,
                "allow_models": "model_a,model_b",
                "block_models": "model_c",
                "subnet": "192.168.0.0/24",
                "Tags": [
                    {"TagName": "department", "TagValue": "engineering", "TagLevel": 3}
                ],
                "quota_plans": ["daily_quota"]
            }
        }
    },
    "Config": {
        "example_product": [
            {
                "Cond": "default_t()",
                "Action": {
                    "Cmd": "CHECK_TOKEN"
                }
            }
        ]
    }
}
```

## 7. 说明

- `Unit = total_token` 时，`Quota` 为整数 Token 数。
- `Unit = RMB` 时，`Quota` 为定点整数，精度为 `1e-8` 元（即 1 单位 = 0.00000001 元）。例如 `90000000` 表示 `0.9` 元。
- BFE 使用 Go 的 `encoding/json` 解析配置文件，字段匹配不区分大小写；但 `QuotaPlan`、`Cond`、`Action`、`Cmd`、`Tags` 等结构体未设置 json tag，因此示例中保留与 Go 字段同名的 CamelCase 写法。若使用 snake_case，部分字段（如 `PassNoQuota`、`RedisKey`、`ExpiredTime`）可能无法被正确解析。
