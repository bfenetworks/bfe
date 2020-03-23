# 配置简介

cluster_table.data配置文件记录各后端集群包含的子集群及实例

# 配置描述

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| Config  | Object<br>各集群信息配置 |
| Config{k} | String<br>集群名称 |
| Config{v} | Object<br>集群详细配置信息 |
| Config{v}.{k} | String<br>子集群名称 |
| Config{v}.{v} | Object<br>子集群包含的实例信息列表 |
| Config{v}.{v}[] | Object<br>实例详细信息 |
| Config{v}.{v}[].Addr | String<br>实例监听地址 |
| Config{v}.{v}[].Port | Integer<br>实例监听端口 |
| Config{v}.{v}[].Weight | Integer<br>实例权重 |
| Config{v}.{v}[].Addr | String<br>实例名称 |

# 配置示例

```
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



