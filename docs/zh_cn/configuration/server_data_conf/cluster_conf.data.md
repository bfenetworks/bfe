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
| BackendConf.TimeoutConnSrv        | Integer<br>连接后端的超时时间，单位是毫秒<br>默认值2000 |
| BackendConf.TimeoutResponseHeader | Integer<br>从后端读响应头的超时时间，单位是毫秒<br>默认值60000 |
| BackendConf.MaxIdleConnsPerHost   | Integer<br>BFE实例与每个后端的最大空闲长连接数<br>默认值2 |
| BackendConf.MaxConnsPerHost   | Integer<br>BFE实例与每个后端的最大长连接数，0代表无限制<br>默认值0 |
| BackendConf.RetryLevel            | Integer<br>请求重试级别。0：连接后端失败时，进行重试；1：连接后端失败、转发GET请求失败时均进行重试<br>默认值0 |
| BackendConf.OutlierDetectionHttpCode            | String<br>后端响应状态码异常检查，""代表不开启检查，"500"表示后端返回500则认为后端失败<br>支持两种格式："\[0-9\]{3}"（如"500"）和"\[0-9\]xx"（如"4xx"）;多个状态码之间使用'&#124;'连接<br>默认值""，不开启后端响应状态码异常检查 |
| BackendConf.FCGIConf              | Object<br>FastCGI 协议的配置                              |
| BackendConf.FCGIConf.Root         | String<br>网站的Root文件夹位置                            |
| BackendConf.FCGIConf.EnvVars      | Map\[string\]string<br>拓展的环境变量                     |

#### 健康检查配置

| 配置项                   | 描述                                                         |
| ------------------------ | ------------------------------------------------------------ |
| CheckConf.Schem         | String<br>健康检查协议，支持HTTP和TCP<br>默认值 HTTP         |
| CheckConf.Uri           | String<br>健康检查请求URI (仅HTTP)<br>默认值 "/health_check" |
| CheckConf.Host          | String<br>健康检查请求HOST (仅HTTP)<br>默认值 ""             |
| CheckConf.StatusCode    | Integer<br>期待返回的响应状态码 (仅HTTP)<br>默认值 200。也可以配置为0，代表任意状态码均符合预期。 |
| CheckConf.FailNum       | Integer<br>健康检查启动阈值（转发请求连续失败FailNum次后，将后端实例置为不可用状态，并启动健康检查）<br>默认值5 |
| CheckConf.SuccNum       | Integer<br>健康检查成功阈值（健康检查连续成功SuccNum次后，将后端实例置为可用状态）<br>默认值1 |
| CheckConf.CheckTimeout  | Integer<br>健康检查的超时时间，单位是毫秒<br>默认值0（无超时）|
| CheckConf.CheckInterval | Integer<br>健康检查的间隔时间，单位是毫秒<br>默认值1000 |

#### GSLB基础配置

| 配置项                           | 描述                                                         |
| -------------------------------- | ------------------------------------------------------------ |
| GslbBasic.CrossRetry             | Integer<br>跨子集群最大重试次数<br>默认值0                   |
| GslbBasic.RetryMax               | Integer<br>子集群内最大重试次数<br>默认值2                   |
| GslbBasic.BalanceMode            | String<br>负载均衡模式(WRR: 加权轮询; WLC: 加权最小连接数)<br>默认值WRR |
| GslbBasic.HashConf               | Object<br>会话保持的HASH策略配置                             |
| GslbBasic.HashConf.HashStrategy  | Integer<br>会话保持的哈希策略。0：ClientIdOnly, 1：ClientIpOnly, 2：ClientIdPreferred，3：RequestURI<br>默认值为1(ClientIpOnly) |
| GslbBasic.HashConf.HashHeader    | String<br>会话保持的hash请求头。可选参数。可配置为能用于唯一区分一个客户端的Header。如果是一个cookie header, 格式为："Cookie:key" |
| GslbBasic.HashConf.SessionSticky | Boolean<br>是否开启会话保持（开启后，可以保证来源于同一个用户的请求可以发送到同一个后端）<br>默认值False。设为False时，会话保持级别为子集群级别。 |

#### 集群基础配置

| 配置项                              | 描述                                                         |
| ----------------------------------- | ------------------------------------------------------------ |
| ClusterBasic.TimeoutReadClient      | Integer<br>读用户请求body的超时时间，单位为毫秒<br>默认值30000 |
| ClusterBasic.TimeoutWriteClient     | Integer<br>写响应的超时时间，单位为毫秒<br>默认值60000       |
| ClusterBasic.TimeoutReadClientAgain | Integer<br>连接闲置超时时间，单位为毫秒<br>默认值60000       |
| ClusterBasic.ReqWriteBufferSize     | Integer<br>请求的写buffer大小，单位为Bytes。默认值512。建议使用默认值。 |
| ClusterBasic.ReqFlushInterval       | Integer<br>刷新请求的间隔时间，单位是毫秒。默认值为0，表示不进行周期性刷新 |
| ClusterBasic.ResFlushInterval       | Integer<br>刷新响应的间隔时间，单位是毫秒。默认值为-1，表示不对响应进行缓存。设置为0表示不进行周期性刷新。建议使用默认值。 |
| ClusterBasic.CancelOnClientClose    | Boolean<br>当服务端正在读后端响应时，如果客户端断连，是否取消该阻塞状态。默认值为false。建议使用默认值。 |

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
                "RetryLevel": 0,
                "OutlierDetectionHttpCode": "5xx|403"
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
                "TimeoutReadClientAgain": 60000,
            }
        },
        "fcgi_cluster_example": {
            "BackendConf": {
                "Protocol": "fcgi",
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "MaxConnsPerHost": 0,
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
                    "HashStrategy": 1,
                    "HashHeader": "Cookie:UID",
                    "SessionSticky": false
                }
            },
            "ClusterBasic": {
                "TimeoutReadClient": 30000,
                "TimeoutWriteClient": 60000,
                "TimeoutReadClientAgain": 60000,
                "ReqWriteBufferSize": 512,
                "ReqFlushInterval": 0,
                "ResFlushInterval": -1,
                "CancelOnClientClose": false
            }
        }
    }
}
```
