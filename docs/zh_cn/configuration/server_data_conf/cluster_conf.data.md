# 简介

cluster_conf.data为集群转发配置文件。

# 配置

## 基础配置

| 配置项  | 类型   | 描述                                                 |
| ------- | ------ | ---------------------------------------------------- |
| Version | String | 配置文件版本                                         |
| Config  | Map&lt;String, ClusterConf&gt; | 集群转发配置参数，key 是集群名称， value是集群转发配置参数 |

## 集群转发配置 ClusterConf

### 后端基础配置 BackendConf

| 配置项                | 类型 | 描述                                                         | 默认值 |
| --------------------- | ---- | ------------------------------------------------------------ | ------ |
| TimeoutConnSrv        | Int  | 连接后端的超时时间，单位是毫秒                               | 2s     |
| TimeoutResponseHeader | Int  | 从后端读响应头的超时时间，单位是毫秒                         | 60s    |
| MaxIdleConnsPerHost   | Int  | BFE实例与每个后端的最大空闲长连接数                          | 2      |
| RetryLevel            | Int  | 请求重试级别。0：连接后端失败时，进行重试；1：连接后端失败、转发GET请求失败时均进行重试 | 0      |

### 健康检查配置 CheckConf

| 配置项        | 类型   | 描述                                                         | 默认值        |
| ------------- | ------ | ------------------------------------------------------------ | ------------- |
| Schem         | String | 健康检查协议，支持HTTP和TCP                                  | HTTP          |
| Uri           | String | 健康检查请求URI (仅HTTP)                                     | /health_check |
| Host          | String | 健康检查请求HOST (仅HTTP)                                    | 空            |
| StatusCode    | Int    | 期待返回的响应状态码 (仅HTTP)                                | 0             |
| FailNum       | Int    | 健康检查启动阈值F（转发请求连续失败F次后，将后端实例置为不可用状态，并启动健康检查） | 5             |
| SuccNum       | Int    | 健康检查成功阈值S（健康检查连续成功S次后，将后端实例置为可用状态） | 1             |
| CheckTimeout  | Int    | 健康检查的超时时间，单位是毫秒                               | 0（无超时）   |
| CheckInterval | Int    | 健康检查的间隔时间，单位是毫秒                               | 1s            |

### GSLB基础配置 GslbBasic

| 配置项      | 类型   | 描述                                                         | 默认值                                            |
| ----------- | ------ | ------------------------------------------------------------ | ------------------------------------------------- |
| CrossRetry  | Int    | 跨子集群最大重试次数                                         | 0                                                 |
| RetryMax    | Int    | 子集群内最大重试次数                                         | 2                                                 |
| BalanceMode | String | 负载均衡模式，默认为WRR<br>-WRR: 加权轮询 <br>- WLC: 加权最小连接数 | WRR                                               |
| HashConf    | Struct | 会话保持的HASH策略配置<br>- HashStrategy: 会话保持的哈希策略。例如：ClientIdOnly, ClientIpOnly, ClientIdPreferred<br>- HashHeader: 会话保持的hash请求头<br>- SessionSticky: 是否开启会话保持 （开启后，可以保证来源于同一个用户的请求可以发送到同一个后端） | -HashStrategy: ClientIpOnly<br>-SessionSticky: 否 |

### 集群基础配置 ClusterBasic

| 配置项                 | 类型 | 描述                                 | 默认值 |
| ---------------------- | ---- | ------------------------------------ | ------ |
| TimeoutReadClient      | Int  | 读用户请求wody的超时时间，单位为毫秒 | 30s    |
| TimeoutWriteClient     | Int  | 写响应的超时时间，单位为毫秒         | 60s    |
| TimeoutReadClientAgain | Int  | 连接闲置超时时间，单位为毫秒         | 60s    |

# 示例

```
{
    "Version": "20190101000000",
    "Config": {
        "cluster_example": {
            "BackendConf": {
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0
            },
            "CheckConf": {
                "Schem": "http",
                "Uri": "/healthcheck",
                "Host": "example.org",
                "StatusCode": 200,
                "FailNum": 10,
                "CheckInterval": 1000
            },
            "GslbBasic": {
                "CrossRetry": 0,
                "RetryMax": 2,
                "HashConf": {
                    "HashStrategy": 0,
                    "HashHeader": "Cookie:UID",
                    "SessionSticky": false
                }
            },
            "ClusterBasic": {
                "TimeoutReadClient": 30000,
                "TimeoutWriteClient": 60000,
                "TimeoutReadClientAgain": 30000,
            }
        }
    }
}
```
