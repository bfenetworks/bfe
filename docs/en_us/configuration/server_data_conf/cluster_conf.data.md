# Cluster Forwarding Configuration

## Configuration Introduction

`cluster_conf.data` is the configuration file for cluster forwarding.

## Configuration Description

### Basic Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Configuration file version | Y | See the [Version](../00-common.md#5-version) type definition | Type is [Version](../00-common.md#5-version) |
| Config | Object | Forwarding configuration parameters for each cluster | Y | Key is the cluster name, value is the cluster forwarding configuration parameters | Non-empty |
| Config[k] | String | Cluster name | Y | Used as the key of Config | Non-empty |
| Config[v] | Object | Cluster forwarding configuration parameters | Y | Contains BackendConf, CheckConf, GslbBasic, ClusterBasic, HTTPSConf, AIConf, etc. | Non-empty |

### Cluster Forwarding Configuration

Note: The following configuration items are located in the namespace `Config[v]`, and the namespace is omitted in the configuration item names.

#### Backend Basic Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ----------------------------------- | --------------- | ---------------------------------------------- | -------- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| BackendConf.Protocol | String | Protocol of the backend service | N | Default value `http` | Only supports `http`, `https`, `fcgi`, `tcp`, `ws`, `h2c` |
| BackendConf.TimeoutConnSrv | Integer | Timeout for connecting to the backend, in milliseconds | N | Default value 2000 | >= 0 |
| BackendConf.TimeoutResponseHeader | Integer | Timeout for reading the response header from the backend, in milliseconds | N | Default value 60000 | >= 0 |
| BackendConf.MaxIdleConnsPerHost | Integer | Maximum number of idle persistent connections between the BFE instance and each backend | N | Default value 2 | >= 0 |
| BackendConf.MaxConnsPerHost | Integer | Maximum number of persistent connections between the BFE instance and each backend; 0 means unlimited | N | Default value 0; 0 means no limit | >= 0 |
| BackendConf.SlowStartTime | Integer | Slow start time for backend instances, in seconds; 0 means disabled | N | Default value 0; 0 means slow start is disabled | >= 0 |
| BackendConf.RetryLevel | Integer | Request retry level | N | Default value 0; 0: retry when connection to backend fails; 1: retry when connection to backend fails or forwarding GET request fails | Only supports 0 or 1 |
| BackendConf.OutlierDetectionHttpCode | String | Backend response status code anomaly check | N | Default value `""`, meaning no check; `"500"` means the backend is considered failed if it returns 500; supports formats `"[0-9]{3}"` (e.g. `"500"`) and `"[0-9]xx"` (e.g. `"4xx"`); multiple patterns can be connected with `&#124;` | Type is [HTTPStatusCodePattern](../00-common.md#9-httpstatuscodepattern); empty string means disabled |
| BackendConf.FCGIConf | Object | FastCGI protocol configuration | N | Effective only when Protocol is `fcgi` | - |
| BackendConf.FCGIConf.Root | String | Root folder location of the website | Conditional | Required when FCGIConf is configured | Non-empty |
| BackendConf.FCGIConf.EnvVars | Map[string]string | Extended environment variables | N | Custom FastCGI environment variables | - |

#### Health Check Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ----------------------------- | ------- | ---------------------------------------------- | -------- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| CheckConf.Schem | String | Health check protocol | N | Default value `HTTP` | Only supports `HTTP`, `HTTPS`, `TCP`, `TLS` |
| CheckConf.HostType | String | Health check request host type | N | Default value `HOST`; `HOST` uses CheckConf.Host, `_ADDR` uses the backend instance address | Only supports `HOST` and `_ADDR` |
| CheckConf.Uri | String | Health check request URI (only HTTP/HTTPS) | N | Default value `"/health_check"` | - |
| CheckConf.Host | String | Health check request HOST (only HTTP/HTTPS) | N | Default value `""` | - |
| CheckConf.StatusCode | Integer | Expected response status code (only HTTP/HTTPS) | N | Default value 0, meaning any status code is acceptable; can also be configured to a specific code such as 200 | >= 0 |
| CheckConf.StatusCodeRange | String | Expected response status code range (only HTTP/HTTPS) | N | See Note 1. StatusCodeRange | Type is [HTTPStatusCodePattern](../00-common.md#9-httpstatuscodepattern) |
| CheckConf.FailNum | Integer | Health check activation threshold | N | After forwarding requests fail consecutively for FailNum times, the backend instance is marked as unavailable and health check is initiated; default value 5 | > 0 |
| CheckConf.SuccNum | Integer | Health check success threshold | N | After health check succeeds consecutively for SuccNum times, the backend instance is marked as available; default value 1 | > 0 |
| CheckConf.CheckTimeout | Integer | Health check timeout, in milliseconds | N | Default value 0 (no timeout) | >= 0 |
| CheckConf.CheckInterval | Integer | Health check interval, in milliseconds | N | Default value 1000 | > 0 |

#### GSLB Basic Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ----------------------------------- | --------- | ---------------------------------------------- | -------- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| GslbBasic.CrossRetry | Integer | Maximum cross-sub-cluster retry count | N | Default value 0 | >= 0 |
| GslbBasic.RetryMax | Integer | Maximum retry count within a sub-cluster | N | Default value 2 | >= 0 |
| GslbBasic.BalanceMode | String | Load balancing mode | N | Default value `WRR` | Only supports `WRR` (Weighted Round Robin), `WLC` (Weighted Least Connections), `EPP` (External Policy-based Load Balancing) |
| GslbBasic.EPPAddr | []String | List of EPP server addresses | Conditional | Effective only when BalanceMode is `EPP` | Non-empty list; each element is a valid address |
| GslbBasic.HashConf | Object | Hash strategy configuration for session persistence | N | - | - |
| GslbBasic.HashConf.HashStrategy | Integer | Hash strategy for session persistence | N | Default value 1 (ClientIpOnly) | Only supports 0 (ClientIdOnly), 1 (ClientIpOnly), 2 (ClientIdPreferred), 3 (RequestURI) |
| GslbBasic.HashConf.HashHeader | String | Hash request header for session persistence | N | Optional; can be configured as a Header that uniquely identifies a client; if it is a cookie header, the format is `"Cookie:key"` | - |
| GslbBasic.HashConf.SessionSticky | Boolean | Whether to enable session persistence | N | Default value `False`; when set to `False`, the session persistence level is at the sub-cluster level | - |

#### Cluster Basic Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| --------------------------------------- | ------- | ---------------------------------------------- | -------- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| ClusterBasic.TimeoutReadClient | Integer | Timeout for reading the client request body, in milliseconds | N | Default value 30000 | >= 0 |
| ClusterBasic.TimeoutWriteClient | Integer | Timeout for writing the response, in milliseconds | N | Default value 60000 | >= 0 |
| ClusterBasic.TimeoutReadClientAgain | Integer | Timeout for idle connections, in milliseconds | N | Default value 60000 | >= 0 |
| ClusterBasic.ReqWriteBufferSize | Integer | Request write buffer size, in Bytes | N | Default value 512; recommended to use the default value | > 0 |
| ClusterBasic.ReqFlushInterval | Integer | Interval for flushing requests, in milliseconds | N | Default value 0, meaning no periodic flushing | >= 0 |
| ClusterBasic.ResFlushInterval | Integer | Interval for flushing responses, in milliseconds | N | Default value -1, meaning no caching of responses; setting to 0 means no periodic flushing; recommended to use the default value | - |
| ClusterBasic.CancelOnClientClose | Boolean | Whether to cancel the blocking state when the client disconnects while the server is reading the backend response | N | Default value `false`; recommended to use the default value | - |
| ClusterBasic.DisableHostHeader | Boolean | Whether to disable the Host header automatically added/overridden by BFE | N | Default value `false` | - |
| ClusterBasic.DisableHealthCheck | Boolean | Whether to disable health check for this cluster | N | Default value `false` | - |

#### Backend Service HTTPS Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ---------------------------------- | --------- | ---------------------------------------------- | -------- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| HTTPSConf.RSHost | String | Hostname of the backend service instance, used to verify the server certificate | N | No default value; must be explicitly configured | Non-empty; must be a valid hostname |
| HTTPSConf.BFEKeyFile | String | Private key file path | Conditional | Required when mutual authentication is supported; private key used by the BFE engine when forwarding HTTPS requests to the backend; must be in PEM format | Type is [FilePath](../00-common.md#3-filepath); required when `RSInsecureSkipVerify=false` and mutual authentication is needed |
| HTTPSConf.BFECertFile | String | Certificate file path | Conditional | Required when mutual authentication is supported; certificate used by the BFE engine when forwarding HTTPS requests to the backend; must be in x509 standard PEM format; each PEM file can only contain one certificate | Type is [FilePath](../00-common.md#3-filepath); required when `RSInsecureSkipVerify=false` and mutual authentication is needed |
| HTTPSConf.RSCAList | []String | Backend server certificate CA list | Conditional | Required when BackendConf.Protocol is `https` and server certificate verification is needed (i.e. RSInsecureSkipVerify is false); if not filled, the system default CA pool is used | Each element type is [FilePath](../00-common.md#3-filepath); must be an x509 standard PEM format certificate |
| HTTPSConf.RSInsecureSkipVerify | Boolean | Server certificate verification switch | N | Default value `false` | - |

#### AI Service Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ----------------------------- | ----------------- | ---------------------------------------------- | -------- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| AIConf.Type | Integer | AI service type | N | Currently reserved; keep it 0 | Only supports 0 |
| AIConf.Key | String | API-Key for the backend large model service | N | If empty, the API-Key is not reset when accessing the backend service and the request's API-Key is retained | - |
| AIConf.ModelMapping | Map[string]string | Mapping from original request model to backend service model | N | When accessing the backend service, the model field in the request will be looked up in this mapping; if matched, the model field in the request will be overwritten | Both keys and values are non-empty |

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

## Notes

### 1. StatusCodeRange

- Response status code range. If StatusCode is configured, this validation condition will be ignored.
- Valid configuration examples:
  1. One of `"3xx"`, `"4xx"`, `"5xx"`
  2. Specific HTTP return codes, consistent with the StatusCode function
  3. The above (1) or (2) connected by the `"|"` symbol, for example:
     - `"503|4xx"`
     - `"501|409|30x"`
