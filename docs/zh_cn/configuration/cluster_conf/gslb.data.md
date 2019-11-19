# 简介

gslb.data配置文件记录各集群内的多个子集群之间分流比例(GSLB)。

# 配置

| 配置项   | 类型   | 描述                                            |
| -------- | ------ | ---------------------------------------------- |
| Clusters | Map<String, GSLBConf> | 各集群中子集群的分流比例；Key代表集群名称，Value代表集群内子集群之间分流比例 |
| Hostname | String | 配置文件生成来源信息                            |
| Ts       | String | 配置文件生成的时间戳                            |

## GSLBConf
Cluster记录集群内子集群之间分流比例，类型为Map<String, Weight>
- Key代表子集群名称（注：保留关键字GSLB_BLACKHOLE代表黑洞子集群，分配到该子集群的流量将被丢弃，用于过载保护）
- Value代表分流权重（注：取值范围0～100；各子集群分流权重之和应等于100）

# 示例

```
{
    "Clusters": {
        "cluster_example": {
            "GSLB_BLACKHOLE": 0,
            "example.bfe.bj": 100
        }
    },
    "Hostname": "gslb-sch.example.com",
    "Ts": "20190101000000"
}
```

