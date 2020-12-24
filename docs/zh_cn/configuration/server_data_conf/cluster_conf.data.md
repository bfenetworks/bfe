# 集群转发配置

## 配置简介

cluster_conf.data为集群转发配置文件。

## 配置描述

### 基础配置

| 配置项     | 描述                           |
| ---------- | ------------------------------ |
| Version    | String<br>配置文件版本         |
| Config     | Object<br>各集群的转发配置参数 |
| Config[k]  | String<br>集群名称             |
| Config[v]  | Object<br>集群转发配置参数     |

### 集群转发配置

注：以下配置项均位于名字空间Config[v], 在配置项名称中已省略

#### 后端基础配置

| 配置项                        | 描述                                                         |
| ----------------------------- | ------------------------------------------------------------ |
| BackendConf.Protocol              | String<br>后端服务的协议，当前支持http和fcgi, 默认值http |
| BackendConf.TimeoutConnSrv        | Integer<br>连接后端的超时时间，单位是毫秒<br>默认值2 |
| BackendConf.TimeoutResponseHeader | Integer<br>从后端读响应头的超时时间，单位是毫秒<br>默认值60 |
| BackendConf.MaxIdleConnsPerHost   | Integer<br>BFE实例与每个后端的最大空闲长连接数<br>默认值2 |
| BackendConf.RetryLevel            | Integer<br>请求重试级别。0：连接后端失败时，进行重试；1：连接后端失败、转发GET请求失败时均进行重试<br>默认值0 |
| BackendConf.FCGIConf              | Object<br>FastCGI 协议的配置                              |
| BackendConf.FCGIConf.Root         | String<br>网站的Root文件夹位置                            |
| BackendConf.FCGIConf.EnvVars      | Map[string]string<br>拓展的环境变量                       |

#### 健康检查配置

| 配置项                   | 描述                                                         |
| ------------------------ | ------------------------------------------------------------ |
| CheckConf.Schem         | String<br>健康检查协议，支持HTTP和TCP<br>默认值 HTTP         |
| CheckConf.Uri           | String<br>健康检查请求URI (仅HTTP)<br>默认值 /health_check   |
| CheckConf.Host          | String<br>健康检查请求HOST (仅HTTP)<br>默认值 ""             |
| CheckConf.StatusCode    | Integer<br>期待返回的响应状态码 (仅HTTP)<br>默认值 0，代表任意状态码 |
| CheckConf.FailNum       | Integer<br>健康检查启动阈值（转发请求连续失败FailNum次后，将后端实例置为不可用状态，并启动健康检查）<br>默认值5 |
| CheckConf.SuccNum       | Integer<br>健康检查成功阈值（健康检查连续成功SuccNum次后，将后端实例置为可用状态）<br>默认值1 |
| CheckConf.CheckTimeout  | Integer<br>健康检查的超时时间，单位是毫秒<br>默认值0（无超时）|
| CheckConf.CheckInterval | Integer<br>健康检查的间隔时间，单位是毫秒<br>默认值1 |

#### GSLB基础配置

| 配置项                           | 描述                                       |
| -------------------------------- | ------------------------------------------ |
| GslbBasic.CrossRetry             | Integer<br>跨子集群最大重试次数<br>默认值0 |
| GslbBasic.RetryMax               | Integer<br>子集群内最大重试次数<br>默认值2 |
| GslbBasic.BalanceMode            | String<br>负载均衡模式(WRR: 加权轮询; WLC: 加权最小连接数)<br>默认值WRR |
| GslbBasic.HashConf               | Object<br>会话保持的HASH策略配置 |
| GslbBasic.HashConf.HashStrategy  | Integer<br>会话保持的哈希策(ClientIdOnly, ClientIpOnly, ClientIdPreferred)<br>默认值ClientIpOnly |
| GslbBasic.HashConf.HashHeader    | String<br>会话保持的hash请求头 |
| GslbBasic.HashConf.SessionSticky | Boolean<br>是否开启会话保持（开启后，可以保证来源于同一个用户的请求可以发送到同一个后端）<br>默认值False |

#### 集群基础配置

| 配置项                              | 描述                                 |
| ----------------------------------- | ------------------------------------ |
| ClusterBasic.TimeoutReadClient      | Integer<br>读用户请求wody的超时时间，单位为毫秒<br>默认值30 |
| ClusterBasic.TimeoutWriteClient     | Integer<br>写响应的超时时间，单位为毫秒<br>默认值60 |
| ClusterBasic.TimeoutReadClientAgain | Integer<br>连接闲置超时时间，单位为毫秒<br>默认值60 |
| ClusterBasic.RequestBuffering       | Boolean<br>异步传输，缓存用户请求body，等待数据读取完毕后向backend转发<br>默认值false |
| ClusterBasic.MaxRequestBodyBytes    | Integer<br>请求body最大缓存大小，单位为字节<br>默认值1073741824(1G) |
| ClusterBasic.MemRequestBodyBytes    | Integer<br>请求body最大内存缓存大小，单位为字节<br>默认值1048576(1M) |
| ClusterBasic.ProxyBuffering         | Boolean<br>异步传输，缓存backend相应body，等待数据读取完毕后向client转发 |
| ClusterBasic.MaxResponseBodyBytes   | Integer<br>响应body最大缓存大小，单位为字节<br>默认值1073741824(1G) |
| ClusterBasic.MemResponseBodyBytes   | Integer<br>响应body最大内存缓存大小，单位为字节<br>默认值1048576(1M) |

## 配置示例

```json
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
        },
        "fcgi_cluster_example": {
            "BackendConf": {
                "Protocol": "fcgi",
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0,
                "FCGIConf": {
                    "Root": "/home/work",
                    "EnvVars": {
                        "VarKey": "VarVal"
                    }    
                }
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
                "ReqWriteBufferSize": 512,
                "ReqFlushInterval": 0,
                "ResFlushInterval": -1,
                "CancelOnClientClose": false,
                "RequestBuffering": true,
                "MaxRequestBodyBytes": 1000,
                "MemRequestBodyBytes": 1000,
                "ProxyBuffering": true,
                "MaxResponseBodyBytes": 1000,
                "MemResponseBodyBytes": 1000
            }
        }
    }
}
```
