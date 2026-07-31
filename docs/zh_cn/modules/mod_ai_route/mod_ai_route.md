# mod_ai_route

## 模块简介

mod_ai_route 用于根据 AI 路由规则，将 AI 请求路由到不同的后端集群和模型。支持基于 apikey、entity 和 global 三种类型的路由表。

## 基础配置

### 配置描述

模块配置文件: conf/mod_ai_route/mod_ai_route.conf

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Basic.RouteRulePath   | String<br>AI 路由规则文件路径（必填） |
| Log.OpenDebug         | Boolean<br>是否开启 debug 日志<br>默认值 False |

### 配置示例

```ini
[Basic]
RouteRulePath = ../data/mod_ai_route/ai_route.data

[Log]
OpenDebug = true
```

## 规则配置

### 配置描述

规则配置文件: ai_route.data

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| route_rules | Object<br>路由表集合 |
| route_rules{k} | String<br>路由表名称 |
| route_rules{v} | Object<br>路由表详细信息 |
| route_rules{v}.Type | String<br>路由表类型，可选值：apikey、entity、global |
| route_rules{v}.Owner | String<br>路由表所有者 |
| route_rules{v}.Rules | []Object<br>路由规则列表 |
| route_rules{v}.Rules[].Name | String<br>规则名称 |
| route_rules{v}.Rules[].Cond | String<br>匹配请求的条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| route_rules{v}.Rules[].Targets | []Object<br>目标后端集群和模型列表 |
| route_rules{v}.Rules[].Targets[].ClusterName | String<br>目标集群名称 |
| route_rules{v}.Rules[].Targets[].Model | String<br>目标模型名称 |
| route_rules{v}.Rules[].Targets[].Weight | Integer<br>权重 |
| route_rules{v}.Rules[].Fallbacks | []Object<br>fallback 目标列表 |
| route_rules{v}.Rules[].Fallbacks[].ClusterName | String<br>fallback 集群名称 |
| route_rules{v}.Rules[].Fallbacks[].Model | String<br>fallback 模型名称 |
| ApikeyRouteTableBindings | Object<br>apikey 到路由表名称列表的绑定关系 |
| ApikeyRouteTableBindings{k} | String<br>apikey |
| ApikeyRouteTableBindings{v} | []String<br>该 apikey 绑定的路由表名称列表 |

### 配置示例

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

## 监控项

| 监控项         | 描述                     |
| -------------- | ------------------------ |
| REQ_TOTAL      | 请求总数                 |
| REQ_HIT_APIKEY | 命中 apikey 路由的请求数 |
| REQ_HIT_ENTITY | 命中 entity 路由的请求数 |
| REQ_HIT_GLOBAL | 命中 global 路由的请求数 |
| REQ_MISS       | 未命中路由的请求数       |
| REQ_FALLBACK   | 命中 fallback 的请求数   |
