# 简介

gslb.data配置文件记录各集群内的多个子集群之间分流比例(GSLB)。

# 配置

| 配置项   | 类型   | 描述                                            |
| -------- | ------ | ----------------------------------------------- |
| Clusters | Struct | 子集群的分流比例 <br>集群 => 子集群 => 分流比例 |
| Hostname | String | 配置文件生成来源信息                            |
| Ts       | String | 配置文件生成的时间戳                            |

注：保留关键字GSLB_BLACKHOLE代表黑洞子集群，分配到该子集群的流量将被丢弃，一般用于过载保护

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

