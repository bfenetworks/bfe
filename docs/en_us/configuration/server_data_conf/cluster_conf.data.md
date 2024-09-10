# Cluster Configuration

## Introduction

cluster_conf.data records the cluster config.

## Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of config file                             |
| Config      | Struct<br>Map data, key is cluster name, value is cluster config detail |
| Config[k]   | String<br>Cluster name                                       |
| Config[v]   | Object<br>Cluster's routing config                           |

### Cluster Config Detail

Notice: the following configs are in namespace Config[v].

#### Backend Config

BackendConf is config for backend.

| Config Item           | Description                                 |
| --------------------- | ------------------------------------------- |
| Protocol              | String<br>Protocol for connecting backend. http and fcgi are supported. Default value is http. |
| TimeoutConnSrv        | Int<br>Timeout for connecting backend, in ms. Default value is 2000. |
| TimeoutResponseHeader | Int<br>Timeout for reading response header, in ms. Default value is 60000. |
| MaxIdleConnsPerHost   | Int<br>Max idle connections to each backend per BFE. Default value is 2. |
| MaxConnsPerHost   | Int<br>Max number of concurrent connections to each backend per BFE. 0 means no limitation. Default value is 0. |
| RetryLevel            | Int<br>Retry level if request fail. 0: retry after connecting backend fails; 1: retry after connecting backend fails or forwarding GET request fails. Default value is 0. |
| BackendConf.OutlierDetectionHttpCode            | String<br>Backend HTTP status code outlier detection. <br>"" means disable detection, "500" means "500" is considered as backend failure. <br>Supports two formats: "\[0-9\]{3}"(e.g "500"), and "\[0-9\]xx"(e.g "4xx"). Multiple status codes are separated by "\|".<br>Default value is "", which means disable the detection. |
| FCGIConf              | Object<br>Conf for FastCGI Protocol                |
| FCGIConf.Root         | String<br>The root folder of the website       |
| FCGIConf.EnvVars      | Map\[string\]string<br>Extra environment variable  |

#### Health Check Config

CheckConf is config of backend check.

| Config Item   | Description                                                  |
| ------------- | ------------------------------------------------------------ |
| Schem         | String<br>Protocol for health check (HTTP/TCP). Default value is http. |
| Uri           | String<br>Uri used in health check (HTTP only). Default value is "/health_check". |
| Host          | String<br>Host used in health check (HTTP only). Default value is "". |
| StatusCode    | Int<br>Expected response code (HTTP only). Default value is 200. And 0 means any response code is considered valid. |
| FailNum       | Int<br>Failure threshold (consecutive failures of forwarded requests), which will trigger BFE to set the backend instance to unavailable state and start the health check. |
| SuccNum       | Int<br>Healthy threshold (consecutive successes of health check request), which will trigger BFE to set the backend instance to available state and stop the health check. |
| CheckTimeout  | Int<br>Timeout for health check, in ms. Default value is 0, which means no timeout. |
| CheckInterval | Int<br>Interval of health check, in ms. Default value is 1000. |

#### GSLB Config

GslbBasic is cluster config for Gslb.

| Config Item           | Description                                                  |
| --------------------- | ------------------------------------------------------------ |
| CrossRetry            | Int<br>Max cross sub-clusters retry times. Default value is 0. |
| RetryMax              | Int<br>Max retry times within same sub-cluster. Default value is 2. |
| BalanceMode           | String<br>Load Balance Mode. Supported mode: WRR(Weighted Round Robin), WLC(Weighted Least Connection). Default value is WRR. |
| HashConf              | Struct<br>Hash config of session persistence<br>             |
| HashConf.HashStrategy | Int<br>Hash Strategy of session persistence. Supported strategies: 0: ClientIdOnly, 1: ClientIpOnly, 2: ClientIdPreferred, 3: RequestURI. Default value is 1 (ClientIpOnly).<br> |
| HashConf.HashHeader   | String<br>HashHeader is an optional request header which represents a unique client. Format for speicial cookie header is "Cookie:Key"<br> |
| HashConf.SessonSticky | Boolean<br>Set SessionSticky to "true" enable sticky session (ensures that all requests from the user during the session are sent to the same backend). If set to "false", the session persistence will be at sub-cluster level. |

#### Cluster Basic Config

ClusterBasic is basic config for cluster.

| Config Item            | Description                                                  |
| ---------------------- | ------------------------------------------------------------ |
| TimeoutReadClient      | Int<br>Timeout for read client body in ms. Default value is 30000. |
| TimeoutWriteClient     | Int<br>Timeout for write response to client. Default value is 60000. |
| TimeoutReadClientAgain | Int<br>Timeout for read client again in ms. Default value is 60000. |
| ReqWriteBufferSize     | Int<br>Write buffer size for request in byte. Default and recommended value is 512. |
| ReqFlushInterval       | Int<br>Interval to flush request in ms. Default and recommended value is 0, means disable periodic flush. |
| ResFlushInterval       | Int<br>Interval to flush response in ms. Default and recommended value is -1, means not to cache response. 0 means disable periodic flush. |
| CancelOnClientClose    | Bool<br>During reading response from backend, cancel the blocking status if client connection disconnected. Default and recommended value is false. |

## Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "cluster_example": {
            "BackendConf": {
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "MaxConnsPerHost": 0,
                "RetryLevel": 0,
                "OutlierDetectionHttpCode": "5xx|400"
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
                "ReqWriteBufferSize": 512,
                "ReqFlushInterval": 0,
                "ResFlushInterval": -1,
                "CancelOnClientClose": false
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
