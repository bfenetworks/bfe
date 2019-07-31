# 简介

route_rule.data 是BFE的分流配置文件。

# 配置

| 配置项      | 类型   | 描述                                                         |
| ----------- | ------ | ------------------------------------------------------------ |
| Version     | String | 配置文件版本                                                 |
| ProductRule | Struct | 产品线的分流规则配置，该配置是个map数据，key是产品线名称，value是分流规则。每个分流规则包括：<br>- Cond: 分流条件<br>- ClusterName: 目的集群 |

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



