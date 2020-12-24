# Cluster Configuration 

## Introduction

cluster_conf.data records the cluster config.

## Configuration

| Config Item | Description                                                   |
| ----------- | ------------------------------------------------------------- |
| Version     | String<br>Verson of config file                                         |
| Config      | Struct<br>Map data, key is cluster name, value is cluster config detail |

### Cluster Config Detail

#### Backend Config

BackendConf is config for backend.

| Config Item           | Description                                 |
| --------------------- | ------------------------------------------- |
| Protocol              | String<br>Protocol for conect backend, supported http and fcgi, default is http |
| TimeoutConnSrv        | Int<br>Timeout for connect backend, in ms          |
| TimeoutResponseHeader | Int<br>Timeout for read response header, in ms     |
| MaxIdleConnsPerHost   | Int<br>Max idle conns to each backend              |
| RetryLevel            | Int<br>Retry level if request fail                 |
| FCGIConf              | Object<br>Conf for FastCGI Protocol                |
| FCGIConf.Root         | String<br>the root folder to the site              |
| FCGIConf.EnvVars      | Map[string]string<br>extra environment variable    |

#### Health Check Config

CheckConf is config of backend check.

| Config Item   | Description                                                 |
| ------------- | ----------------------------------------------------------- |
| Schem         | String<br>Protocol for health check (HTTP/TCP)                        |
| Uri           | String<br>Uri used in health check (HTTP)                             |
| Host          | String<br>If check request use special host header (HTTP)             |
| StatusCode    | Int<br>Expected response code, default value is 200 (HTTP)         |
| FailNum       | Int<br>Unhealthy threshold (consecutive failures of check request) |
| SuccNum       | Int<br>Healthy threshold (consecutive successes of normal request) |
| CheckTimeout  | Int<br>Timeout for health check, in ms                             |
| CheckInterval | Int<br>Interval of health check, in ms                             |

#### GSLB Config

GslbBasic is cluster config for Gslb.

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| CrossRetry  | Int<br>Cross sub-clusters retry times                               |
| RetryMax    | Int<br>Inner cluster retry times                                    |
| BalanceMode | String<br>BalanceMode, default WRR                                     |
| HashConf    | Struct<br>Hash config about load balabnce<br>- HashStrategy: HashStrategy is hash strategy for subcluster-level load balance. Such as ClientIdOnly, ClientIpOnly, ClientIdPreferred<br>- HashHeader: HashHeader is an optional request header which represents a unique client. Format for speicial cookie header is "Cookie:Key"<br>- SessionSticky: SessionSticky enable sticky session (ensures that all requests from the user during the session are sent to the same backend) |

#### Cluster Basic Config

ClusterBasic is basic config for cluster.

| Config Item            | Description                                                  |
| ---------------------- | ------------------------------------------------------------ |
| TimeoutReadClient      | Int<br>Timeout for read client body in ms                           |
| TimeoutWriteClient     | Int<br>Timeout for write response to client                         |
| TimeoutReadClientAgain | Int<br>Timeout for read client again in ms                          |
| ReqWriteBufferSize     | Int<br>Write buffer size for request in byte                        |
| ReqFlushInterval       | Int<br>Interval to flush request in ms. if zero, disable periodic flush |
| ResFlushInterval       | Int<br>Interval to flush response in ms. if zero, disable periodic flush |
| CancelOnClientClose    | Bool<br>Cancel blocking operation on server if client connection disconnected |
| ClusterBasic.RequestBuffering       | Bool<br>Asynchronous transport, caches the user request Body, waits for data to be read and then forwards to Backend<br>default false |
| ClusterBasic.MaxRequestBodyBytes    | Int<br>Request the maximum body cache size in bytes<br>default 1073741824(1G) |
| ClusterBasic.MemRequestBodyBytes    | Int<br>Request the maximum body memory cache size in bytes<br>default 1048576(1M) |
| ClusterBasic.ProxyBuffering         | Bool<br>Asynchronous transport, cache Backend corresponding Body, wait for data read and then forward to client |
| ClusterBasic.MaxResponseBodyBytes   | Int<br>Maximum cache size in bytes for response body<br>default 1073741824(1G) |
| ClusterBasic.MemResponseBodyBytes   | Int<br>Response Body maximum memory cache size in bytes<br>default 1048576(1M) |

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
