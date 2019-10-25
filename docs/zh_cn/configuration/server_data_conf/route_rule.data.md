# 简介

route_rule.data 是BFE的分流配置文件。

# 配置

| 配置项      | 类型   | 描述                                                         |
| ----------- | ------ | ------------------------------------------------------------ |
| Version     | String | 配置文件版本                                                 |
| ProductRule | Map&lt;String, Array&lt;RouteRule&gt;&gt; | 产品线的分流规则配置，key是产品线名称，value是分流规则表，包含多条有序分流规则 |

## RouteRule
分流规则包含[分流条件]](../../condition/condition_grammar.md)及目的集群：
| 配置项      | 类型   | 描述                                                         |
| ----------- | ------ | ------------------------------------------------------------ |
| Cond     | String | 分流条件 |
| ClusterName | String | 目的集群名称 |


# 示例

```
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



