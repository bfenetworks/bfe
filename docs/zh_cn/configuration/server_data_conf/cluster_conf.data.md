# 集群转发配置

## 配置简介

cluster_conf.data为集群转发配置文件。

## 配置描述

### 基础配置

| 配置项     | 类型   | 参数含义                 | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ---------- | ------ | ------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version    | String | 配置文件版本             | Y    | 参见 [Version](../00-common.md#5-配置文件版本version) 类型定义  | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| Config     | Object | 各集群的转发配置参数     | Y    | 键为集群名称，值为集群转发配置参数                           | 非空                                                         |
| Config[k]  | String | 集群名称                 | Y    | 作为 Config 的键                                             | 非空                                                         |
| Config[v]  | Object | 集群转发配置参数         | Y    | 包含 BackendConf、CheckConf、GslbBasic、ClusterBasic、HTTPSConf、AIConf 等 | 非空                                                         |

### 集群转发配置

注：以下配置项均位于名字空间Config[v], 在配置项名称中已省略

#### 后端基础配置

| 配置项                              | 类型           | 参数含义                                       | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ----------------------------------- | -------------- | ---------------------------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| BackendConf.Protocol                | String         | 后端服务的协议                                 | N    | 默认值`http`                                                 | 仅支持 `http`、`https`、`fcgi`、`tcp`、`ws`、`h2c`          |
| BackendConf.TimeoutConnSrv          | Integer        | 连接后端的超时时间，单位是毫秒                 | N    | 默认值2000                                                   | >= 0                                                         |
| BackendConf.TimeoutResponseHeader   | Integer        | 从后端读响应头的超时时间，单位是毫秒           | N    | 默认值60000                                                  | >= 0                                                         |
| BackendConf.MaxIdleConnsPerHost     | Integer        | BFE实例与每个后端的最大空闲长连接数            | N    | 默认值2                                                      | >= 0                                                         |
| BackendConf.MaxConnsPerHost         | Integer        | BFE实例与每个后端的最大长连接数                | N    | 默认值0；0代表无限制                                         | >= 0                                                         |
| BackendConf.SlowStartTime           | Integer        | 后端实例慢启动时间，单位为秒                   | N    | 默认值0；0表示不开启慢启动                                   | >= 0                                                         |
| BackendConf.RetryLevel              | Integer        | 请求重试级别                                   | N    | 默认值0；0：连接后端失败时重试；1：连接后端失败、转发GET请求失败时均重试 | 仅支持 0 或 1                                                |
| BackendConf.OutlierDetectionHttpCode | String        | 后端响应状态码异常检查                         | N    | 默认值 `""`，表示不开启检查；`"500"` 表示后端返回500则认为后端失败 | 类型为 [HTTPStatusCodePattern](../00-common.md#9-http-状态码模式httpstatuscodepattern)；为空字符串表示不开启 |
| BackendConf.FCGIConf                | Object         | FastCGI 协议的配置                             | N    | 仅当 Protocol 为 `fcgi` 时生效                               | -                                                            |
| BackendConf.FCGIConf.Root           | String         | 网站的Root文件夹位置                           | 条件 | FCGIConf 配置时必填                                          | 非空                                                         |
| BackendConf.FCGIConf.EnvVars        | Map[string]string | 拓展的环境变量                              | N    | 自定义 FastCGI 环境变量                                      | -                                                            |

#### 健康检查配置

| 配置项                        | 类型    | 参数含义                                       | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ----------------------------- | ------- | ---------------------------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| CheckConf.Schem               | String  | 健康检查协议                                   | N    | 默认值 `HTTP`                                                | 仅支持 `HTTP`、`HTTPS`、`TCP`、`TLS`                         |
| CheckConf.HostType            | String  | 健康检查请求Host类型                           | N    | 默认值 `HOST`；`HOST` 使用 CheckConf.Host，`ADDR` 使用后端实例地址 | 仅支持 `HOST`、`ADDR`                                        |
| CheckConf.Uri                 | String  | 健康检查请求URI                                | N    | 默认值 `"/health_check"`                                     | -                                                            |
| CheckConf.Host                | String  | 健康检查请求HOST                               | N    | 默认值 `""`                                                  | -                                                            |
| CheckConf.StatusCode          | Integer | 期待返回的响应状态码                           | N    | 默认值 0，代表任意状态码均符合预期；也可配置为具体状态码如 200 | >= 0                                                         |
| CheckConf.StatusCodeRange     | String  | 期待返回的响应状态码范围                       | N    | 具体参见注解「1. StatusCodeRange」                           | 类型为 [HTTPStatusCodePattern](../00-common.md#9-http-状态码模式httpstatuscodepattern) |
| CheckConf.FailNum             | Integer | 健康检查启动阈值                               | N    | 转发请求连续失败 FailNum 次后，将后端实例置为不可用状态，并启动健康检查；默认值5 | > 0                                                          |
| CheckConf.SuccNum             | Integer | 健康检查成功阈值                               | N    | 健康检查连续成功 SuccNum 次后，将后端实例置为可用状态；默认值1 | > 0                                                          |
| CheckConf.CheckTimeout        | Integer | 健康检查的超时时间，单位是毫秒                 | N    | 默认值0，表示无超时                                          | >= 0                                                         |
| CheckConf.CheckInterval       | Integer | 健康检查的间隔时间，单位是毫秒                 | N    | 默认值1000                                                   | > 0                                                          |

#### GSLB基础配置

| 配置项                              | 类型      | 参数含义                                       | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ----------------------------------- | --------- | ---------------------------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| GslbBasic.CrossRetry                | Integer   | 跨子集群最大重试次数                           | N    | 默认值0                                                      | >= 0                                                         |
| GslbBasic.RetryMax                  | Integer   | 子集群内最大重试次数                           | N    | 默认值2                                                      | >= 0                                                         |
| GslbBasic.BalanceMode               | String    | 负载均衡模式                                   | N    | 默认值`WRR`                                                  | 仅支持 `WRR`（加权轮询）、`WLC`（加权最小连接数）、`EPP`（基于外部策略的负载均衡） |
| GslbBasic.EPPAddr                   | []String  | EPP服务端地址列表                              | 条件 | 仅当 BalanceMode 为 `EPP` 时生效                             | 非空列表；每个元素为有效地址                                 |
| GslbBasic.HashConf                  | Object    | 会话保持的HASH策略配置                         | N    | -                                                            | -                                                            |
| GslbBasic.HashConf.HashStrategy     | Integer   | 会话保持的哈希策略                             | N    | 默认值为1（ClientIpOnly）                                    | 仅支持 0（ClientIdOnly）、1（ClientIpOnly）、2（ClientIdPreferred）、3（RequestURI） |
| GslbBasic.HashConf.HashHeader       | String    | 会话保持的hash请求头                           | N    | 可选参数；可配置为能用于唯一区分一个客户端的Header；Cookie header 格式为 `"Cookie:key"` | -                                                            |
| GslbBasic.HashConf.SessionSticky    | Boolean   | 是否开启会话保持                               | N    | 默认值`False`；设为 `False` 时，会话保持级别为子集群级别     | -                                                            |

#### 集群基础配置

| 配置项                                  | 类型    | 参数含义                                       | 必填 | 补充描述                                                     | 合法性条件                                                   |
| --------------------------------------- | ------- | ---------------------------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| ClusterBasic.TimeoutReadClient          | Integer | 读用户请求body的超时时间，单位为毫秒           | N    | 默认值30000                                                  | >= 0                                                         |
| ClusterBasic.TimeoutWriteClient         | Integer | 写响应的超时时间，单位为毫秒                   | N    | 默认值60000                                                  | >= 0                                                         |
| ClusterBasic.TimeoutReadClientAgain     | Integer | 连接闲置超时时间，单位为毫秒                   | N    | 默认值60000                                                  | >= 0                                                         |
| ClusterBasic.ReqWriteBufferSize         | Integer | 请求的写buffer大小，单位为Bytes                | N    | 默认值512；建议使用默认值                                    | > 0                                                          |
| ClusterBasic.ReqFlushInterval           | Integer | 刷新请求的间隔时间，单位是毫秒                 | N    | 默认值为0，表示不进行周期性刷新                              | >= 0                                                         |
| ClusterBasic.ResFlushInterval           | Integer | 刷新响应的间隔时间，单位是毫秒                 | N    | 默认值为-1，表示不对响应进行缓存；设置为0表示不进行周期性刷新；建议使用默认值 | -                                                            |
| ClusterBasic.CancelOnClientClose        | Boolean | 当服务端正在读后端响应时，如果客户端断连，是否取消该阻塞状态 | N    | 默认值为`false`；建议使用默认值                              | -                                                            |
| ClusterBasic.DisableHostHeader          | Boolean | 是否禁用由BFE自动添加/覆盖的Host请求头         | N    | 默认值为`false`                                              | -                                                            |
| ClusterBasic.DisableHealthCheck         | Boolean | 是否禁用该集群的健康检查                       | N    | 默认值为`false`                                              | -                                                            |

#### 后端服务HTTPS配置

| 配置项                             | 类型      | 参数含义                                       | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ---------------------------------- | --------- | ---------------------------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| HTTPSConf.RSHost                   | String    | 后端服务实例的hostname                         | N    | 用来验证服务端证书；无默认值，需显式配置                     | 非空；须为有效主机名                                         |
| HTTPSConf.BFEKeyFile               | String    | 私钥文件路径                                   | 条件 | 支持双向认证时必填；BFE引擎向后端转发https请求时使用的私钥；须为pem格式 | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；`RSInsecureSkipVerify=false` 且需要双向认证时必填 |
| HTTPSConf.BFECertFile              | String    | 证书文件路径                                   | 条件 | 支持双向认证时必填；须为符合x509标准的pem格式，每个pem文件只能包含一张证书 | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；`RSInsecureSkipVerify=false` 且需要双向认证时必填 |
| HTTPSConf.RSCAList                 | []String  | 后端服务端证书CA列表                           | 条件 | `BackendConf.Protocol` 为 `https` 且需要验证服务端证书时必填；不填则使用系统默认CA池 | 每个元素类型为 [FilePath](../00-common.md#3-文件路径filepath)；须为符合x509标准的pem格式证书 |
| HTTPSConf.RSInsecureSkipVerify     | Boolean   | 服务端证书验证开关                             | N    | 默认值为`false`                                              | -                                                            |

#### AI服务配置

| 配置项                        | 类型              | 参数含义                                       | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ----------------------------- | ----------------- | ---------------------------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| AIConf.Type                   | Integer           | AI服务类型                                     | N    | 当前保留字段，请保持为0                                      | 仅支持 0                                                     |
| AIConf.Key                    | String            | 后端大模型服务的API-Key                        | N    | 空字符串表示访问后端服务时不重置API-Key，仍保持请求的API-Key | -                                                            |
| AIConf.ModelMapping           | Map[string]string | 原请求model -> 后端服务的model 的映射关系      | N    | 访问后端服务时将根据请求的 model 字段查找此映射关系，命中则重写请求的 model 字段 | 键值均非空                                                   |

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
                "TimeoutReadClientAgain": 60000
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
        },
        "ai_cluster_example": {
            "BackendConf": {
                "Protocol": "https",
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0
            },
            "CheckConf": {
                "Schem": "https",
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
                "ReqWriteBufferSize": 512,
                "ReqFlushInterval": 0,
                "ResFlushInterval": -1,
                "CancelOnClientClose": false
            },
            "AIConf": {
                "Type": 0,
                "Key": "sk-example-api-key",
                "ModelMapping": {
                    "gpt-4": "backend-gpt-4-model"
                }
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
