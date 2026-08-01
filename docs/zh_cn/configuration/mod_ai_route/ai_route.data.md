# mod_ai_route 规则配置

## 配置简介

`ai_route.data` 是 `mod_ai_route` 模块的规则配置文件。

## 配置描述

| 配置项                                       | 类型    | 参数含义                             | 必填 | 补充描述                                                     | 合法性条件                                                   |
| -------------------------------------------- | ------- | ------------------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version                                      | String  | 配置文件版本                         | Y    | 通常采用时间戳格式，如 `20260720150000`                      | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| route_rules                                  | Object  | 路由表集合                           | Y    | -                                                            | -                                                            |
| route_rules{k}                               | String  | 路由表名称                           | Y    | -                                                            | -                                                            |
| route_rules{v}                               | Object  | 路由表详细信息                       | Y    | -                                                            | -                                                            |
| route_rules{v}.type                          | String  | 路由表类型                           | Y    | 可选值：`apikey`、`entity`、`global`                         | 取值范围为 `apikey`、`entity`、`global`                      |
| route_rules{v}.owner                         | String  | 路由表所有者                         | Y    | -                                                            | -                                                            |
| route_rules{v}.rules                         | Array   | 路由规则列表                         | Y    | -                                                            | -                                                            |
| route_rules{v}.rules[]                       | Object  | 路由规则                             | Y    | -                                                            | -                                                            |
| route_rules{v}.rules[].name                  | String  | 规则名称                             | Y    | -                                                            | -                                                            |
| route_rules{v}.rules[].Cond                  | String  | 匹配请求的条件                       | Y    | 语法详见 [Condition](../../condition/condition_grammar.md)   | -                                                            |
| route_rules{v}.rules[].targets               | Array   | 目标后端集群和模型列表               | Y    | -                                                            | -                                                            |
| route_rules{v}.rules[].targets[]             | Object  | 目标后端集群和模型                   | Y    | -                                                            | -                                                            |
| route_rules{v}.rules[].targets[].ClusterName | String  | 目标集群名称                         | Y    | -                                                            | -                                                            |
| route_rules{v}.rules[].targets[].Model       | String  | 目标模型名称                         | N    | 为空表示不指定模型                                           | -                                                            |
| route_rules{v}.rules[].targets[].Weight      | Integer | 权重                                 | Y    | -                                                            | 类型为 [Weight](../00-common.md#8-权重weight)；同一规则下权重之和须大于 0 |
| route_rules{v}.rules[].fallbacks             | Array   | fallback 目标列表                    | N    | 为空表示无 fallback                                          | -                                                            |
| route_rules{v}.rules[].fallbacks[]           | Object  | fallback 目标                        | N    | -                                                            | -                                                            |
| route_rules{v}.rules[].fallbacks[].ClusterName | String | fallback 集群名称                   | Y    | -                                                            | -                                                            |
| route_rules{v}.rules[].fallbacks[].Model     | String  | fallback 模型名称                    | N    | 为空表示不指定模型                                           | -                                                            |
| ApikeyRouteTableBindings                     | Object  | apikey 到路由表名称列表的绑定关系    | N    | 为空表示无 apikey 绑定                                       | -                                                            |
| ApikeyRouteTableBindings{k}                  | String  | apikey                               | Y    | -                                                            | -                                                            |
| ApikeyRouteTableBindings{v}                  | Array   | 该 apikey 绑定的路由表名称列表       | Y    | 按顺序匹配对应路由表                                         | -                                                            |
| ApikeyRouteTableBindings{v}[]                | String  | 路由表名称                           | Y    | 须为 `route_rules` 中已定义的名称                           | -                                                            |

## 配置示例

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
