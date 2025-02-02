# Cluster Forwarding Configuration

## Configuration Introduction

`cluster_conf.data` is the configuration file for cluster forwarding.

## Configuration Description

### Basic Configuration

| Configuration Item | Description |
| ------------------ | ----------- |
| Version            | String<br>Configuration file version |
| Config             | Object<br>Forwarding configuration parameters for each cluster |
| Config[k]          | String<br>Cluster name |
| Config[v]          | Object<br>Cluster forwarding configuration parameters |

### Cluster Forwarding Configuration

Note: The following configuration items are located in the namespace `Config[v]`, and the namespace is omitted in the configuration item names.

#### Backend Basic Configuration

| Configuration Item | Description |
| ------------------ | ----------- |
| BackendConf.Protocol | String<br>Protocol of the backend service, currently supports http/https and fcgi, default value is http |
| BackendConf.TimeoutConnSrv | Integer<br>Timeout for connecting to the backend, in milliseconds<br>Default value 2000 |
| BackendConf.TimeoutResponseHeader | Integer<br>Timeout for reading the response header from the backend, in milliseconds<br>Default value 60000 |
| BackendConf.MaxIdleConnsPerHost | Integer<br>Maximum number of idle persistent connections between the BFE instance and each backend<br>Default value 2 |
| BackendConf.MaxConnsPerHost | Integer<br>Maximum number of persistent connections between the BFE instance and each backend, 0 means no limit<br>Default value 0 |
| BackendConf.RetryLevel | Integer<br>Request retry level. 0: Retry when connection to backend fails; 1: Retry when connection to backend fails or forwarding GET request fails<br>Default value 0 |
| BackendConf.OutlierDetectionHttpCode | String<br>Backend response status code anomaly check, "" means no check, "500" means backend is considered failed if it returns 500<br>Supports two formats: "\[0-9\]{3}" (e.g., "500") and "\[0-9\]xx" (e.g., "4xx"); multiple status codes can be connected using '&#124;'<br>Default value "", no backend response status code anomaly check |
| BackendConf.FCGIConf | Object<br>FastCGI protocol configuration |
| BackendConf.FCGIConf.Root | String<br>Root folder location of the website |
| BackendConf.FCGIConf.EnvVars | Map\[string\]string<br>Extended environment variables |

#### Health Check Configuration

| Configuration Item | Description |
| ------------------ | ----------- |
| CheckConf.Schem | String<br>Health check protocol, supports HTTP/HTTPS/TCP/TLS<br>Default value HTTP |
| CheckConf.Uri | String<br>Health check request URI (only HTTP/HTTPS)<br>Default value "/health_check" |
| CheckConf.Host | String<br>Health check request HOST (only HTTP/HTTPS)<br>Default value "" |
| CheckConf.StatusCode | Integer<br>Expected response status code (only HTTP/HTTPS)<br>Default value 200. Can also be configured as 0, meaning any status code is acceptable. |
| CheckConf.StatusCodeRange | String<br>Expected response status code (only HTTP/HTTPS)<br>See: Note 1. StatusCodeRange |
| CheckConf.FailNum | Integer<br>Health check activation threshold (after forwarding requests fail consecutively for FailNum times, the backend instance is marked as unavailable and health check is initiated)<br>Default value 5 |
| CheckConf.SuccNum | Integer<br>Health check success threshold (after health check succeeds consecutively for SuccNum times, the backend instance is marked as available)<br>Default value 1 |
| CheckConf.CheckTimeout | Integer<br>Health check timeout, in milliseconds<br>Default value 0 (no timeout) |
| CheckConf.CheckInterval | Integer<br>Health check interval, in milliseconds<br>Default value 1000 |

#### GSLB Basic Configuration

| Configuration Item | Description |
| ------------------ | ----------- |
| GslbBasic.CrossRetry | Integer<br>Maximum cross-sub-cluster retry count<br>Default value 0 |
| GslbBasic.RetryMax | Integer<br>Maximum retry count within a sub-cluster<br>Default value 2 |
| GslbBasic.BalanceMode | String<br>Load balancing mode (WRR: Weighted Round Robin; WLC: Weighted Least Connections)<br>Default value WRR |
| GslbBasic.HashConf | Object<br>Hash strategy configuration for session persistence |
| GslbBasic.HashConf.HashStrategy | Integer<br>Hash strategy for session persistence. 0: ClientIdOnly, 1: ClientIpOnly, 2: ClientIdPreferred, 3: RequestURI<br>Default value 1 (ClientIpOnly) |
| GslbBasic.HashConf.HashHeader | String<br>Hash request header for session persistence. Optional. Can be configured as a Header that uniquely identifies a client. If it is a cookie header, the format is: "Cookie:key" |
| GslbBasic.HashConf.SessionSticky | Boolean<br>Whether to enable session persistence (when enabled, requests from the same user can be sent to the same backend)<br>Default value False. When set to False, the session persistence level is at the sub-cluster level. |

#### Cluster Basic Configuration

| Configuration Item | Description |
| ------------------ | ----------- |
| ClusterBasic.TimeoutReadClient | Integer<br>Timeout for reading the client request body, in milliseconds<br>Default value 30000 |
| ClusterBasic.TimeoutWriteClient | Integer<br>Timeout for writing the response, in milliseconds<br>Default value 60000 |
| ClusterBasic.TimeoutReadClientAgain | Integer<br>Timeout for idle connections, in milliseconds<br>Default value 60000 |
| ClusterBasic.ReqWriteBufferSize | Integer<br>Request write buffer size, in Bytes. Default value 512. Recommended to use the default value. |
| ClusterBasic.ReqFlushInterval | Integer<br>Interval for flushing requests, in milliseconds. Default value 0, meaning no periodic flushing |
| ClusterBasic.ResFlushInterval | Integer<br>Interval for flushing responses, in milliseconds. Default value -1, meaning no caching of responses. Setting to 0 means no periodic flushing. Recommended to use the default value. |
| ClusterBasic.CancelOnClientClose | Boolean<br>Whether to cancel the blocking state when the client disconnects while the server is reading the backend response. Default value false. Recommended to use the default value. |

#### Backend Service HTTPS Configuration

| Configuration Item | Description |
| ------------------ | ----------- |
| HTTPSConf.RSHost | String<br>Hostname of the backend service instance, used to verify the server certificate.<br>Default value: Host field in the frontend request header. |
| HTTPSConf.BFEKeyFile | String<br>Private key file path, required when mutual authentication is supported<br>Private key used by the BFE engine when forwarding HTTPS requests to the backend. The private key file must be in PEM format |
| HTTPSConf.BFECertFile | String<br>Certificate file path, required when mutual authentication is supported<br>Certificate used by the BFE engine when forwarding HTTPS requests to the backend. The certificate file must be in x509 standard PEM format, and each PEM file can only contain one certificate |
| HTTPSConf.RSCAList | []String<br>Required when BackendConf.Protocol is https and server certificate verification is needed (i.e., RSInsecureSkipVerify is false). If not filled, the system default CA pool is used. List items are certificate file paths. Certificate files must be in x509 standard PEM format. Multiple CA certificates in the CA trust chain can be combined into one PEM file. |
| HTTPSConf.RSInsecureSkipVerify | Boolean<br>Server certificate verification switch<br>true: Do not verify, false: Verify (default) |

## Configuration Example

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

## Notes

### 1. StatusCodeRange

- Response status code range. If StatusCode is configured, this validation condition will be ignored.
- Valid configuration examples:
  1. One of `"3xx"`, `"4xx"`, `"5xx"`
  2. Specific HTTP return codes, consistent with the StatusCode function
  3. The above (1) or (2) connected by the `"|"` symbol, for example:
     - `"503|4xx"`
     - `"501|409|30x"`