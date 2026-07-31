# mod_ai_route

## Introduction

mod_ai_route routes AI requests to different backend clusters and models based on AI routing rules. It supports three types of routing tables: apikey, entity, and global.

## Module Configuration

### Description

Module configuration file: conf/mod_ai_route/mod_ai_route.conf

| Config Item | Description |
| ----------- | ----------- |
| Basic.RouteRulePath | String<br>Path of AI routing rule file (required) |
| Log.OpenDebug | Boolean<br>Whether to enable debug logs<br>Default False |

### Example

```ini
[Basic]
RouteRulePath = ../data/mod_ai_route/ai_route.data

[Log]
OpenDebug = true
```

## Rule Configuration

### Description

Rule configuration file: ai_route.data

| Config Item | Description |
| ----------- | ----------- |
| Version | String<br>Version of config file |
| route_rules | Object<br>Collection of routing tables |
| route_rules{k} | String<br>Routing table name |
| route_rules{v} | Object<br>Detailed routing table information |
| route_rules{v}.Type | String<br>Routing table type: apikey, entity, global |
| route_rules{v}.Owner | String<br>Owner of routing table |
| route_rules{v}.Rules | []Object<br>List of routing rules |
| route_rules{v}.Rules[].Name | String<br>Rule name |
| route_rules{v}.Rules[].Cond | String<br>Condition to match the request, see [Condition](../../condition/condition_grammar.md) |
| route_rules{v}.Rules[].Targets | []Object<br>List of target backend clusters and models |
| route_rules{v}.Rules[].Targets[].ClusterName | String<br>Target cluster name |
| route_rules{v}.Rules[].Targets[].Model | String<br>Target model name |
| route_rules{v}.Rules[].Targets[].Weight | Integer<br>Weight |
| route_rules{v}.Rules[].Fallbacks | []Object<br>List of fallback targets |
| route_rules{v}.Rules[].Fallbacks[].ClusterName | String<br>Fallback cluster name |
| route_rules{v}.Rules[].Fallbacks[].Model | String<br>Fallback model name |
| ApikeyRouteTableBindings | Object<br>Binding from apikey to list of routing table names |
| ApikeyRouteTableBindings{k} | String<br>apikey |
| ApikeyRouteTableBindings{v} | []String<br>List of routing table names bound to the apikey |

### Example

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

## Metrics

| Metric | Description |
| ------ | ----------- |
| REQ_TOTAL | Total count of requests |
| REQ_HIT_APIKEY | Count of requests hitting apikey routing |
| REQ_HIT_ENTITY | Count of requests hitting entity routing |
| REQ_HIT_GLOBAL | Count of requests hitting global routing |
| REQ_MISS | Count of requests missing all routing |
| REQ_FALLBACK | Count of requests hitting fallback |
