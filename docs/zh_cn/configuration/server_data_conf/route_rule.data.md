# 分流规则配置

## 配置简介

route_rule.data 是BFE的分流配置文件。

## 配置描述

| 配置项                       | 描述                                                       |
| ---------------------------- | ---------------------------------------------------------- |
| Version                      | String<br>配置文件版本                                     |
| ProductRule                  | Object<br>各产品线的分流规则配置                           |
| ProductRule[k]               | String<br>产品线名称                                       |
| ProductRule[v]               | Object<br>分流规则表，包含多条有序分流规则                 |
| ProductRule[v][]             | Object<br>分流规则                                         |
| ProductRule[v][].Cond        | String<br>分流条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| ProductRule[v][].ClusterName | Object<br>目的集群                                         |

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
