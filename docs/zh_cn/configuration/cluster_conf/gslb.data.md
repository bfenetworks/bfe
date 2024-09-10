# 子集群负载均衡配置

## 配置简介

gslb.data配置文件记录各集群内的多个子集群之间分流比例(GSLB)。

## 配置描述

| 配置项   | 描述                                            |
| -------- | ---------------------------------------------- |
| Hostname | String<br>配置文件生成来源信息                            |
| Ts       | String<br>配置文件生成的时间戳                            |
| Clusters | Object<br>各集群中子集群的分流权重 |
| Clusters{k} | String<br>集群名称 |
| Clusters{v} | Object<br>集群内子集群之间分流权重 |
| Clusters{v}{k} | String<br>子集群名称<br>保留GSLB_BLACKHOLE代表黑洞子集群，分配到该子集群的流量将被丢弃，用于过载保护 |
| Clusters{v}{v} | Integer<br>子集群承接流量的权重<br>子集群承接流量的权重取值范围 0～100,各子集群分流权重之和应等于 100 |

## 配置示例

```json
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
