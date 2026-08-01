# 分流规则配置

## 配置简介

route_rule.data 是BFE的分流配置文件。

## 配置描述

| 配置项                       | 类型     | 参数含义                     | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ---------------------------- | -------- | ---------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version                      | String   | 配置文件版本                 | Y    | 参见 [Version](../00-common.md#5-配置文件版本version) 类型定义  | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| ProductRule                  | Object   | 各产品线的分流规则配置       | N    | 键为产品线名称，值为分流规则表                               | -                                                            |
| ProductRule[k]               | String   | 产品线名称                   | 条件 | ProductRule 非空时必填                                       | 非空                                                         |
| ProductRule[v]               | []Object | 分流规则表                   | 条件 | ProductRule 非空时必填；包含多条有序分流规则                 | 非空                                                         |
| ProductRule[v][]             | Object   | 分流规则                     | Y    | 包含 Cond 和 ClusterName                                     | 非空                                                         |
| ProductRule[v][].Cond        | String   | 分流条件                     | Y    | 语法详见 [Condition](../../condition/condition_grammar.md)   | 非空；须为合法 BFE 条件表达式                                |
| ProductRule[v][].ClusterName | String   | 目的集群                     | Y    | 转发命中的目标集群名称                                       | 非空                                                         |
| BasicRule                    | Object   | 基础分流规则（可选）         | N    | 基于 Host+Path 的静态路由表，按产品线组织                    | -                                                            |
| BasicRule[k]                 | String   | 产品线名称                   | 条件 | BasicRule 非空时必填                                         | 非空                                                         |
| BasicRule[v][]               | []Object | 基础路由规则                 | 条件 | BasicRule 非空时必填                                         | 非空                                                         |
| BasicRule[v][].Hostname      | String   | 匹配的Host                   | Y    | 支持通配符                                                   | 非空                                                         |
| BasicRule[v][].Path          | String   | 匹配的Path                   | Y    | URL 路径                                                     | 非空                                                         |
| BasicRule[v][].ClusterName   | String   | 目的集群                     | Y    | 转发命中的目标集群名称                                       | 非空                                                         |

## 配置示例

```json
{
    "Version": "20190101000000",
    "ProductRule": {
        "example_product": [
            {
                "Cond": "req_host_in(\"example.org\")",
                "ClusterName": "cluster_example1"
            },
            {
                "Cond": "default_t()",
                "ClusterName": "cluster_example2"
            }
        ]
    }
}
```
