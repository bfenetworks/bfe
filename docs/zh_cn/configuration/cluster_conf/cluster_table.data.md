# 简介

cluster_table.data配置文件记录各后端集群包含的子集群及实例

# 配置

| 配置项  | 类型   | 描述                                                           |
| ------- | ------ | -------------------------------------------------------------- |
| Version | String | 配置版本                                                       |
| Config  | Map<String, ClusterBackend> | 各集群信息配置。Key代表集群名称，Value代表集群信息 |

## ClusterBackend
ClusterBackend记录集群信息，类型为Map<String, Array<BackendConf>>
- Key代表子集群名称
- Value代表子集群包含的实例信息列表
    
## BackendConf
BackendConf记录后端实例信息

| 配置项  | 类型   | 描述         |
| ------- | ------ | ----------- |
| Addr   | String | 实例监听地址  |
| Port   | Int    | 实例监听端口  |
| Weight | Int    | 实例权重     |
| Name   | String | 实例名称     |


# 示例

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



