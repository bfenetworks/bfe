# 实例负载均衡配置

## 配置简介

cluster_table.data配置文件记录各后端集群包含的子集群及实例

## 配置描述

### 基础配置

| 配置项       | 类型   | 参数含义             | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ------------ | ------ | -------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version      | String | 配置文件版本         | Y    | 参见 [Version](../00-common.md#5-配置文件版本version) 类型定义  | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| Config       | Object | 各集群信息配置       | Y    | 键为集群名称，值为子集群配置                                 | 非空                                                         |
| Config{k}    | String | 集群名称             | Y    | 作为 Config 的键                                             | 非空                                                         |
| Config{v}    | Object | 集群详细配置信息     | Y    | 键为子集群名称，值为实例列表                                 | 非空                                                         |
| Config{v}{k} | String | 子集群名称           | Y    | 作为 Config{v} 的键                                          | 非空                                                         |
| Config{v}{v} | []Object | 子集群配置信息       | Y    | 包含多个实例配置                                             | 非空；每个子集群至少需要一个 `Weight > 0` 的实例             |

### 实例配置

| 配置项 | 类型    | 参数含义         | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ------ | ------- | ---------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Addr   | String  | 实例监听地址     | Y    | 参见 [Hostname](../00-common.md#6-主机名hostname) 类型定义      | 类型为 [Hostname](../00-common.md#6-主机名hostname)             |
| Port   | Integer | 实例监听端口     | Y    | 参见 [Port](../00-common.md#1-网络端口port) 类型定义            | 类型为 [Port](../00-common.md#1-网络端口port)                   |
| Weight | Integer | 实例权重         | Y    | 参见 [Weight](../00-common.md#8-权重weight) 类型定义            | 类型为 [Weight](../00-common.md#8-权重weight)；须 >= 0          |
| Name   | String  | 实例名称         | Y    | 实例标识                                                     | 非空                                                         |

## 配置示例

```json
{
    "Config": {
        "cluster_example": {
            "example.bfe.bj": [
                {
                    "Addr": "10.199.189.26",
                    "Name": "example_hostname",
                    "Port": 10257,
                    "Weight": 10
                }
            ]
        }
    }, 
    "Version": "20190101000000"
}
```
