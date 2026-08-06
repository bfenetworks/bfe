# mod_ai_route Rule Configuration

## Introduction

`ai_route.data` is the rule configuration file of the `mod_ai_route` module, used to configure AI routing tables and apikey-to-routing-table bindings.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | Usually a timestamp, e.g. `20260720150000` | Type is [Version](../00-common.md#5-version) |
| route_rules | Object | Collection of routing tables | Y | - | - |
| route_rules{k} | String | Routing table name | Y | - | - |
| route_rules{v} | Object | Detailed routing table information | Y | - | - |
| route_rules{v}.type | String | Routing table type | Y | Valid values: `apikey`, `entity`, `global` | Value must be one of `apikey`, `entity`, `global` |
| route_rules{v}.owner | String | Owner of routing table | Y | - | - |
| route_rules{v}.rules | Array | List of routing rules | Y | - | - |
| route_rules{v}.rules[] | Object | A routing rule | Y | - | - |
| route_rules{v}.rules[].name | String | Rule name | Y | - | - |
| route_rules{v}.rules[].Cond | String | Condition to match the request | Y | Syntax see [Condition](../../condition/condition_grammar.md) | - |
| route_rules{v}.rules[].targets | Array | List of target backend clusters and models | Y | - | - |
| route_rules{v}.rules[].targets[] | Object | A target backend cluster and model | Y | - | - |
| route_rules{v}.rules[].targets[].ClusterName | String | Target cluster name | Y | - | - |
| route_rules{v}.rules[].targets[].Model | String | Target model name | N | Empty means no specific model | - |
| route_rules{v}.rules[].targets[].Weight | Integer | Weight | Y | - | Type is [Weight](../00-common.md#8-weight); the sum of weights under the same rule must be greater than 0 |
| route_rules{v}.rules[].fallbacks | Array | List of fallback targets | N | Empty means no fallback | - |
| route_rules{v}.rules[].fallbacks[] | Object | A fallback target | N | - | - |
| route_rules{v}.rules[].fallbacks[].ClusterName | String | Fallback cluster name | Y | - | - |
| route_rules{v}.rules[].fallbacks[].Model | String | Fallback model name | N | Empty means no specific model | - |
| ApikeyRouteTableBindings | Object | Binding from apikey to list of routing table names | N | Empty means no apikey binding | - |
| ApikeyRouteTableBindings{k} | String | apikey | Y | - | - |
| ApikeyRouteTableBindings{v} | Array | List of routing table names bound to the apikey | Y | Match routing tables in order | - |
| ApikeyRouteTableBindings{v}[] | String | Routing table name | Y | Must be a name defined in `route_rules` | - |

## Configuration Example

```json
{
    "Version": "20260720150000",
    "route_rules": {
        "apikey_ak_user_a": {
            "type": "apikey",
            "owner": "ak_user_a",
            "rules": [
                {
                    "name": "user_a-rule1",
                    "Cond": "req_host_in(\"api.example.org\")",
                    "targets": [
                        {
                            "ClusterName": "cluster_deepseek_a",
                            "Model": "deepseek-v4-pro",
                            "Weight": 70
                        },
                        {
                            "ClusterName": "cluster_deepseek_b",
                            "Model": "deepseek-v4-pro",
                            "Weight": 30
                        }
                    ],
                    "fallbacks": [
                        {
                            "ClusterName": "cluster_deepseek_c",
                            "Model": "deepseek-v3.2"
                        }
                    ]
                }
            ]
        },
        "entity_dept_ai": {
            "type": "entity",
            "owner": "dept_ai",
            "rules": [
                {
                    "name": "dept_ai-default",
                    "Cond": "default_t()",
                    "targets": [
                        {
                            "ClusterName": "cluster_dept_ai",
                            "Model": "",
                            "Weight": 100
                        }
                    ],
                    "fallbacks": []
                }
            ]
        },
        "global_default": {
            "type": "global",
            "owner": "global",
            "rules": [
                {
                    "name": "global-default",
                    "Cond": "default_t()",
                    "targets": [
                        {
                            "ClusterName": "cluster_global",
                            "Model": "",
                            "Weight": 100
                        }
                    ],
                    "fallbacks": []
                }
            ]
        }
    },
    "ApikeyRouteTableBindings": {
        "ak_user_a": [
            "apikey_ak_user_a",
            "global_default"
        ],
        "ak_user_b": [
            "entity_dept_ai",
            "global_default"
        ]
    }
}
```
