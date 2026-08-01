# 名字规则配置

## 配置简介

name_conf.data记录了服务名字和服务实例的映射关系。

## 配置描述

| 配置项             | 类型     | 参数含义                 | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ------------------ | -------- | ------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version            | String   | 配置文件版本             | Y    | 参见 [Version](../00-common.md#5-配置文件版本version) 类型定义  | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| Config             | Object   | 名字和实例的映射关系     | Y    | 键为服务名称，值为实例信息列表                               | 非空                                                         |
| Config[k]          | String   | 服务名称                 | Y    | 作为 Config 的键                                             | 非空                                                         |
| Config[v]          | []Object | 实例信息列表             | Y    | 该服务名称对应的所有实例                                     | 非空                                                         |
| Config[v][]        | Object   | 实例信息                 | Y    | 包含 Host、Port、Weight                                      | 非空                                                         |
| Config[v][].Host   | String   | 实例地址                 | Y    | 参见 [IPAddr](../00-common.md#7-ip-地址ipaddr) 类型定义         | 类型为 [IPAddr](../00-common.md#7-ip-地址ipaddr)                |
| Config[v][].Port   | Integer  | 实例端口                 | Y    | 参见 [Port](../00-common.md#1-网络端口port) 类型定义            | 类型为 [Port](../00-common.md#1-网络端口port)                   |
| Config[v][].Weight | Integer  | 实例权重                 | Y    | 参见 [Weight](../00-common.md#8-权重weight) 类型定义            | 类型为 [Weight](../00-common.md#8-权重weight)；须 >= 0          |

**注意：** `name_conf.data` 为可选配置。仅在 `bfe.conf` 的 `[Server]` 段配置了 `NameConf` 时才会被加载。

## 配置示例

```json
{
    "Version": "20190101000000",
    "Config": {
        "example.redis.cluster": [
            {
                "Host": "192.168.1.1",
                "Port": 6439,
                "Weight": 10
            }
        ]
    }
}
```
