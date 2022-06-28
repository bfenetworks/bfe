# 名字规则配置

## 配置简介

name_conf.data记录了服务名字和服务实例的映射关系。

## 配置描述

| 配置项             | 描述                           |
| ------------------ | ------------------------------ |
| Version            | String<br>配置文件版本         |
| Config             | Object<br>名字和实例的映射关系 |
| Config[k]          | String<br>集群名称             |
| Config[v]          | Object<br>实例信息列表         |
| Config[v][]        | Object<br>实例信息             |
| Config[v][].Host   | String<br>实例地址             |
| Config[v][].Port   | Integer<br>实例端口            |
| Config[v][].Weight | Integer<br>实例权重            |

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
