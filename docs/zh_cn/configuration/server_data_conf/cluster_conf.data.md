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
| BackendConf.Protocol              | String<br>后端服务的协议，当前支持http/https和fcgi, 默认值http |
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
| CheckConf.Schem         | String<br>健康检查协议，支持HTTP/HTTPS/TCP/TLS<br>默认值 HTTP         |
| CheckConf.Uri           | String<br>健康检查请求URI (仅HTTP/HTTPS)<br>默认值 "/health_check" |
| CheckConf.Host          | String<br>健康检查请求HOST (仅HTTP/HTTPS)<br>默认值 ""             |
| CheckConf.StatusCode    | Integer<br>期待返回的响应状态码 (仅HTTP/HTTPS)<br>默认值 200。也可以配置为0，代表任意状态码均符合预期。 |
| CheckConf.StatusCodeRange | String<br>期待返回的响应状态码 (仅HTTP/HTTPS)<br> 具体参见: 注解 1. StatusCodeRange|
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

#### 后端服务HTTPS配置

| 配置项                             | 描述                                                         |
| --------------------------------- | ------------------------------------------------------------ |
| HTTPSConf.RSHost                  | String<br>后端服务实例的hostname，用来验证服务端证书。<br>默认值：前端请求头中的Host字段。|
| HTTPSConf.BFEKeyFile              | String<br>私钥文件路径，支持双向认证时必填<br>BFE引擎向后端转发https请求时使用的私钥。私钥文件必须是pem格式 |
| HTTPSConf.BFECertFile             | String<br>证书文件路径,支持双向认证时必填<br>BFE引擎向后端转发https请求时使用的证书，证书文件必须是符合x509标准的pem格式，且每个pem文件中只能包含一张证书 |
| HTTPSConf.RSCAList                | []String<br>BackendConf.Protocol为https，并且需要验证服务端的证书（即RSInsecureSkipVerify为false）时必填，如果不填则使用系统默认CA池。列表项为证书文件路径，证书文件必须是符合x509标准的pem格式证书，允许将CA信任链中的多个CA证书合入一个pem文件中。|
| HTTPSConf.RSInsecureSkipVerify    | Boolean<br>服务端证书验证开关<br>true：不验证，false：验证（默认）|

#### AI服务配置

| 配置项                             | 描述                                                         |
| --------------------------------- | ------------------------------------------------------------ |
| AIConf.Key                  | String<br>后端大模型服务的API-Key<br>空 - 访问后端服务时不重置API-Key，仍保持请求的API-Key |
| ModelMapping                | Map\[string\]string<br>原请求model -> 后端服务的model 的映射关系。访问后端服务时将根据请求的 model 字段查找此映射关系，命中的话则重写请求的 model 字段|

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
        "https_cluster_example": {
            "BackendConf": {
                "Protocol": "https",
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0
            },
            "CheckConf": {
                "Schem": "https",
                "Uri": "/",
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
                "CancelOnClientClose": false
            },
            "HTTPSConf":{
                "RSHost": "www.example.org",
                "BFEKeyFile": "../conf/tls_conf/backend_rs/r_bfe_dev_prv.pem",
                "BFECertFile": "../conf/tls_conf/backend_rs/r_bfe_dev.crt",
                "RSCAList": [
                    "../conf/tls_conf/backend_rs/bfe_r_ca.crt",
                    "../conf/tls_conf/backend_rs/bfe_i_ca.crt"
                ],
                "RSInsecureSkipVerify": false
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

## 注解

### 1. StatusCodeRange 

- 响应状态码范围。如果配置了StatusCode，则会忽略此验证条件
- 合法的配置项举例：
  1. `"3xx"`, `"4xx"`, `"5xx"` 其中之一
  2. 特定的HTTP返回码，与StatusCode功能一致
  3. `"|"` 符号连接的上述 (1)或 (2) 例如： 
     - `"503|4xx"`
     - `"501|409|30x"`
