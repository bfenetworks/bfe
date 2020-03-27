# 配置简介

gslb.data配置文件记录各集群内的多个子集群之间分流比例(GSLB)。

# 配置描述

| 配置项   | 描述                                            |
| -------- | ---------------------------------------------- |
| Hostname | String<br>配置文件生成来源信息                            |
| Ts       | String<br>配置文件生成的时间戳                            |
| Clusters | Object<br>各集群中子集群的分流比例 |
| Clusters{k} | String<br>集群名称 |
| Clusters{v} | Object<br>集群内子集群之间分流比例 |
| Clusters{v}.{k} | String<br>子集群名称 |
| Clusters{v}.{v} | Integer<br>子集群承接流量的比例 |
 * 注：
    * 子集群名称保留关键字 GSLB_BLACKHOLE 代表黑洞子集群，分配到该子集群的流量将被丢弃，用于过载保护
    * 子集群承接流量的比例取值范围 0～100,各子集群分流权重之和应等于 100 
# 配置示例

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

