# 子集群负载均衡配置

## 配置简介

gslb.data配置文件记录各集群内的多个子集群之间分流比例(GSLB)。

## 配置描述

| 配置项          | 类型    | 参数含义                 | 必填 | 补充描述                                                     | 合法性条件                                                   |
| --------------- | ------- | ------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Hostname        | String  | 配置文件生成来源信息     | Y    | 标识该配置文件由哪个主机/系统生成                            | 非空                                                         |
| Ts              | String  | 配置文件生成的时间戳     | Y    | 标识配置文件生成时间                                         | 非空                                                         |
| Clusters        | Object  | 各集群中子集群的分流权重 | Y    | 键为集群名称，值为子集群权重映射                             | 非空                                                         |
| Clusters{k}     | String  | 集群名称                 | Y    | 作为 Clusters 的键                                           | 非空                                                         |
| Clusters{v}     | Object  | 集群内子集群之间分流权重 | Y    | 键为子集群名称，值为权重值                                   | 非空                                                         |
| Clusters{v}{k}  | String  | 子集群名称               | Y    | 作为 Clusters{v} 的键                                        | 非空；保留 `GSLB_BLACKHOLE` 代表黑洞子集群，分配到该子集群的流量将被丢弃，用于过载保护 |
| Clusters{v}{v}  | Integer | 子集群承接流量的权重     | Y    | 参见 [Weight](../00-common.md#8-权重weight) 类型定义            | 类型为 [Weight](../00-common.md#8-权重weight)；各子集群正权重之和须大于 0 |

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
