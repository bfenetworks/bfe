# Introduction

cluster_conf.data records the cluster config.

# Configuration

| Config Item | Type   | Description                                                   |
| ----------- | ------ | ------------------------------------------------------------- |
| Version     | String | Verson of config file                                         |
| Config      | Struct | Map data, key is cluster name, value is cluster config detail |

## Cluster Config Detail

### Backend Config

BackendConf is config for backend.

| Config Item           | Type | Description                                 |
| --------------------- | ---- | ------------------------------------------- |
| TimeoutConnSrv        | Int  | Timeout for connect backend, in ms          |
| TimeoutResponseHeader | Int  | Timeout for read response header, in ms     |
| MaxIdleConnsPerHost   | Int  | Max idle conns to each backend              |
| RetryLevel            | Int  | Retry level if request fail                 |

### Health Check Config

CheckConf is config of backend check.

| Config Item   | Type   | Description                                                 |
| ------------- | ------ | ----------------------------------------------------------- |
| Schem         | String | Protocol for health check (HTTP/TCP)                        |
| Uri           | String | Uri used in health check (HTTP)                             |
| Host          | String | If check request use special host header (HTTP)             |
| StatusCode    | Int    | Expected response code, default value is 200 (HTTP)         |
| FailNum       | Int    | Unhealthy threshold (consecutive failures of check request) |
| SuccNum       | Int    | Healthy threshold (consecutive successes of normal request) |
| CheckTimeout  | Int    | Timeout for health check, in ms                             |
| CheckInterval | Int    | Interval of health check, in ms                             |

### GSLB Config

GslbBasic is cluster config for Gslb.

| Config Item | Type   | Description                                                  |
| ----------- | ------ | ------------------------------------------------------------ |
| CrossRetry  | Int    | Cross sub-clusters retry times                               |
| RetryMax    | Int    | Inner cluster retry times                                    |
| BalanceMode | String | BalanceMode, default WRR                                     |
| HashConf    | Struct | Hash config about load balabnce<br>- HashStrategy: HashStrategy is hash strategy for subcluster-level load balance. Such as ClientIdOnly, ClientIpOnly, ClientIdPreferred<br>- HashHeader: HashHeader is an optional request header which represents a unique client. Format for speicial cookie header is "Cookie:Key"<br>- SessionSticky: SessionSticky enable sticky session (ensures that all requests from the user during the session are sent to the same backend) |

### Cluster Basic Config

ClusterBasic is basic config for cluster.

| Config Item            | Type | Description                                                  |
| ---------------------- | ---- | ------------------------------------------------------------ |
| TimeoutReadClient      | Int  | Timeout for read client body in ms                           |
| TimeoutWriteClient     | Int  | Timeout for write response to client                         |
| TimeoutReadClientAgain | Int  | Timeout for read client again in ms                          |
| ReqWriteBufferSize     | Int  | Write buffer size for request in byte                        |
| ReqFlushInterval       | Int  | Interval to flush request in ms. if zero, disable periodic flush |
| ResFlushInterval       | Int  | Interval to flush response in ms. if zero, disable periodic flush |
| CancelOnClientClose    | Bool | Cancel blocking operation on server if client connection disconnected |

# Example

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
                "ReqWriteBufferSize": 512,
                "ReqFlushInterval": 0,
                "ResFlushInterval": -1,
                "CancelOnClientClose": false
            }
        }
    }
}
```
