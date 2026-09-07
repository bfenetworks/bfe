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
| GslbBasic.RetryMax | Integer | Maximum retry count within a sub-cluster | N | Default value 2; when `BalanceMode` is `EPP` it also bounds retries on the EPP scheduling path | >= 0 |
| GslbBasic.BalanceMode | String | Load balancing mode | N | Default value `WRR` | `WRR` (Weighted Round Robin), `WLC` (Weighted Least Connections), `EPP` (external scheduling via ext-proc, see [EPP Scheduling Configuration](#epp-scheduling-configuration-balancemode--epp)) |
| GslbBasic.HashConf | Object | Hash strategy configuration for session persistence | N | Ineffective when `BalanceMode` is `EPP` (backends are chosen by EPP); kept to ease mode rollback | - |
| GslbBasic.HashConf.HashStrategy | Integer | Hash strategy for session persistence | N | Default value 1 (ClientIpOnly) | Only supports 0 (ClientIdOnly), 1 (ClientIpOnly), 2 (ClientIdPreferred), 3 (RequestURI) |
| GslbBasic.HashConf.HashHeader | String | Hash request header for session persistence | N | Optional; can be configured as a Header that uniquely identifies a client; if it is a cookie header, the format is `"Cookie:key"` | - |
| GslbBasic.HashConf.SessionSticky | Boolean | Whether to enable session persistence | N | Default value `False`; when set to `False`, the session persistence level is at the sub-cluster level | - |
| GslbBasic.EPPAddr | []String | Ordered EPP server address list | Conditional | Required when `BalanceMode` is `EPP`; element `[0]` = primary, `[1]` = backup (see [EPPAddr semantics](#eppaddr-semantics)) | Non-empty list; each element is `host:port`; no duplicates allowed |
| GslbBasic.EPPCheck | Object | EPP health check and failover hysteresis parameters; effective only when `BalanceMode` is `EPP`, enabled with defaults when omitted | N | - | See [EPPCheck elements](#eppcheck-elements) |
| GslbBasic.EPPTimeout | Object | EPP call timeout parameters; effective only when `BalanceMode` is `EPP` | N | - | See [EPPTimeout elements](#epptimeout-elements) |
| GslbBasic.EPPTLS | Object | EPP connection TLS parameters; effective only when `BalanceMode` is `EPP`; defaults to TLS without certificate verification (compatible with legacy deployments) | N | - | See [EPPTLS elements](#epptls-elements) |
| GslbBasic.EPPBreaker | Object | EPP call circuit breaker parameters; effective only when `BalanceMode` is `EPP`, enabled with defaults when omitted | N | - | See [EPPBreaker elements](#eppbreaker-elements) |

##### EPP Scheduling Configuration (BalanceMode = "EPP")

When `BalanceMode` is `EPP`, BFE delegates scheduling decisions to an EPP service over the [ext-proc](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_proc/v3/ext_proc.proto) gRPC protocol: BFE carries the cluster name in the request message (`llm-d.ai/inference-pool` metadata), and EPP returns the selected backend address in the response dynamic metadata `envoy.lb -> x-gateway-destination-endpoint`. When EPP is unavailable, BFE falls back to local load balancing (WRR) per the existing degradation semantics. EPP-related metrics are exposed at `/monitor/epp_metrics`.

###### EPPAddr semantics

`EPPAddr` is an **ordered** address list:

- `[0]` is the **primary** EPP instance of this cluster; `[1]` is the **backup** EPP instance of the same instance group; more than 2 entries are reserved for scaling and are consumed in order.
- Requests go to `[0]` first; the active index only moves to a later address after consecutive health check failures reach the threshold (hysteresis, see [EPPCheck](#eppcheck-elements)).
- A single-element list is valid: no backup instance (test / single-instance-group setups); instance failure degrades to local load balancing.
- The list is usually derived and distributed by ai-gateway-api according to the "instance group primary/backup assignment".

###### EPPCheck elements

```json
{
    "Disabled": false,
    "CheckInterval": "2s",
    "FailThreshold": 3,
    "Cooldown": "45s",
    "SuccessThreshold": 2
}
```

| Field | Type | Required | Description | Validity Condition |
|------|------|------|------|------------|
| Disabled | Boolean | N | Disable background health checks; default `false`; when enabled, only per-request error-driven retry remains (not recommended for production) | - |
| CheckInterval | String | N | Health probe interval (gRPC health, `grpc.health.v1`, probed towards EPPAddr); default `"2s"` | Valid duration string, > 0 |
| FailThreshold | Integer | N | Failover to the next address after this many consecutive failures of the active address; default `3` | > 0 |
| Cooldown | String | N | Cooldown after failover; no failback within this period (anti-flapping); default `"45s"` | Valid duration string, > 0 |
| SuccessThreshold | Integer | N | After cooldown, a higher-priority address must pass this many consecutive probes before failback; default `2` | > 0 |

###### EPPTimeout elements

```json
{
    "Connect": "500ms",
    "Call": "3s"
}
```

| Field | Type | Required | Description | Validity Condition |
|------|------|------|------|------------|
| Connect | String | N | Timeout for establishing the gRPC connection/stream; default `"500ms"` | Valid duration string, > 0 |
| Call | String | N | Timeout for the first message round trip (RequestHeaders Send+Recv); a timeout counts as a failed EPP call and follows the failure fallback path; default `"3s"` | Valid duration string, > 0 |

###### EPPTLS elements

```json
{
    "Insecure": false,
    "CAFile": "/bfe/conf/epp/epp_ca.crt"
}
```

| Field | Type | Required | Description | Validity Condition |
|------|------|------|------|------------|
| Insecure | Boolean | N | Skip verification of the EPP server certificate; default `false`; `true` is for test environments only | - |
| CAFile | String | Conditional | CA file path for verifying the EPP server certificate; required when `Insecure` is `false` | [FilePath](../00-common.md#3-file-pathfilepath); must be readable at load time |

Note: EPP connections always use **TLS** transport. When `EPPTLS` is omitted entirely, BFE keeps TLS but skips certificate verification (compatible with pre-upgrade EPP deployments; a migration warning is logged). Production should configure `EPPTLS` explicitly to enable verification. The CA certificate file can be distributed to a fixed path via the server_data_conf extra_files channel.

###### EPPBreaker elements

```json
{
    "Disabled": false,
    "WindowSize": 100,
    "MinVolume": 20,
    "ErrorRatePercent": 50,
    "OpenTimeout": "30s"
}
```

| Field | Type | Required | Description | Validity Condition |
|------|------|------|------|------------|
| Disabled | Boolean | N | Disable the circuit breaker; default `false` | - |
| WindowSize | Integer | N | Sliding window size (number of recent call results); default `100` | >= 1 |
| MinVolume | Integer | N | Minimum call volume before evaluation; default `20` | >= 1 and <= `WindowSize` |
| ErrorRatePercent | Integer | N | Error rate (percent) at which the breaker opens (OPEN), short-circuiting all EPP calls and degrading to local load balancing; default `50` | [1, 100] |
| OpenTimeout | String | N | After OPEN lasts this long the breaker turns half-open (probe requests allowed); a successful probe closes the breaker and clears the window, a failed one re-opens it; default `"30s"` | Valid duration string, > 0 |

Configuration hot-reload does not reset the OPEN state (avoiding a thundering herd on config reload).

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
| ----------------------------------- | ----------------- | ---------------------------------------------- | -------- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| AIConf.Type | Integer | AI service type | N | Currently reserved; keep it 0 | Only supports 0 |
| AIConf.Provider | String | Provider name of this cluster in `model_prices` | N | Automatically populated by ai-gateway-api based on the OpenAPI `llm_config.provider`; used for cost statistics | - |
| AIConf.Keys | []Object | API-Key list for the backend large model service | N | Empty array means no API-Key is injected when accessing the backend service and the request's API-Key is retained; keys are selected by weighted random | See the "AIConf.Keys elements" table below |
| AIConf.KeyPolicy | Object | API-Key selection policy and retry/backoff configuration | N | Takes effect in multi-Key scenarios; backoff logic does not take effect with single Key or no Key | See the "AIConf.KeyPolicy elements" table below |
| AIConf.ModelMapping | Map[string]string | Mapping from original request model to backend service model | N | When accessing the backend service, the model field in the request will be looked up in this mapping; if matched, the model field in the request will be overwritten | Both keys and values are non-empty |
| AIConf.MatchPrefix | String | Provider/model prefix to match | N | e.g. `openrouter/`; must end with `/`; used for aggregator providers such as OpenRouter | Required when `StripPrefix=true` |
| AIConf.StripPrefix | Boolean | Whether to strip the prefix specified by `MatchPrefix` | N | When `true`, the prefix is removed from the request model field before forwarding to the backend; when `false`, the prefix is only used as a routing marker and not stripped | Defaults to `false` |
| AIConf.ModelTable | Object | Model pricing table of this cluster | N | Automatically populated by ai-gateway-api by querying `model_prices` based on `Provider`; currency is fixed to `RMB` for now | See the "AIConf.ModelTable elements" table below |

##### AIConf.Keys elements

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------- | ------- | ------------------ | -------- | ------------------------------------------------ | ---------- |
| AIConf.Keys[i].Name | String | API-Key name/identifier | Y | Used for logging, monitoring and operations identification | Non-empty |
| AIConf.Keys[i].Key | String | API-Key value | Y | Secret key used for backend authentication | Non-empty |
| AIConf.Keys[i].Weight | Integer | Weight | Y | Used for weighted random selection; range is `[0,100]`; `0` means no traffic is received | `[0,100]`; total weight of multiple keys must be 100 |

##### AIConf.KeyPolicy elements

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------------------------- | ------- | -------------------- | -------- | ------------------------------------------------------------ | -------------------------------- |
| AIConf.KeyPolicy.Strategy | String | Key selection strategy | N | Currently only supports `weighted_random` | Only supports `weighted_random` |
| AIConf.KeyPolicy.MaxRetries | Integer | Total additional retry count | N | Maximum retry count excluding the first selection in one `aiClusterInvoke` call; `0` means no retry | >= 0 |
| AIConf.KeyPolicy.RetryBackoffInitial | Integer | Initial backoff time, in milliseconds | N | Backoff time for the first retry | >= 0 |
| AIConf.KeyPolicy.RetryBackoffMax | Integer | Maximum backoff time, in milliseconds | N | Upper limit of backoff time | >= 0, and must be >= RetryBackoffInitial |
| AIConf.KeyPolicy.SessionAffinity | Boolean | Whether to enable session-level API-Key affinity based on Redis + `ClientKeyId` | N | Default `false`; when enabled, requests from the same `ClientKeyId` are bound to the same API-Key | - |
| AIConf.KeyPolicy.SessionAffinityTTL | Integer | Idle timeout of the `ClientKeyId -> KeyName` binding in Redis, in seconds | N | Default `600`; the TTL is refreshed on each binding hit, so the binding persists as long as the session keeps sending requests | > 0 |
| AIConf.KeyPolicy.SessionAffinityRedisPrefix | String | Prefix of the Redis binding key | N | Default `"bfe:ai:key_affinity"` | Non-empty |
| AIConf.KeyPolicy.SessionAffinityPenaltyEnable | Boolean | Whether to enable Key penalty: skip Keys that recently returned 429/401/403 | N | Default `true` | - |

**Session-level Key Affinity Notes:**

- When enabled, BFE uses `AiBasicInfo.ClientKeyId` as the session identifier and maintains a binding `{prefix}:{cluster_name}:{client_key_id} -> <key_name>` in Redis.
- Subsequent requests with the same `ClientKeyId` prefer the bound Key; on each hit, BFE refreshes the binding TTL via `Expire`, so the binding persists as long as the session keeps sending requests.
- `SessionAffinityTTL` is the **idle timeout**: the binding is released automatically only when no request arrives within the TTL window.
- If the bound Key is penalized, deleted, or has weight `0`, a new Key is selected and the binding is updated.
- If Redis is unavailable, the affinity logic gracefully degrades to weighted random selection without affecting request success rate.
- If `Keys` contains only one valid Key, it is returned directly without accessing Redis.

##### AIConf.ModelTable elements

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------------------- | -------- | ------------------ | -------- | ----------------------------------- | ---------- |
| AIConf.ModelTable.Currency | String | Currency type | Y | Fixed to `RMB` in v0.4 | - |
| AIConf.ModelTable.Models | []Object | Model pricing entry list | Y | Each entry corresponds to a model and its price/limit | See the "AIConf.ModelTable.Models elements" table below |

##### AIConf.ModelTable.Models elements

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ---------------------------------------------- | ----------------- | ------------------ | -------- | ----------------------------------- | ---------- |
| AIConf.ModelTable.Models[i].Provider | String | Provider name | Y | - | Non-empty |
| AIConf.ModelTable.Models[i].Model | String | Model name | Y | Used to match the `target_model` in the request | Non-empty |
| AIConf.ModelTable.Models[i].BaseModel | String | Normalized model name | Y | - | Non-empty |
| AIConf.ModelTable.Models[i].Mode | String | Request mode | N | e.g. `chat` | - |
| AIConf.ModelTable.Models[i].Capabilities | []String | Capability list | N | e.g. `["chat", "reasoning"]` | - |
| AIConf.ModelTable.Models[i].SupportedParameters | []String | Supported request parameter list | N | e.g. `["temperature", "max_tokens"]` | - |
| AIConf.ModelTable.Models[i].Limits | Map[string]Integer | Limit object | N | e.g. `context_window`, etc. | - |
| AIConf.ModelTable.Models[i].Prices | Map[string]Number | Price object | N | e.g. `input_cost_per_token`, etc. | - |

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
                "Provider": "deepseek",
                "MatchPrefix": "openrouter/",
                "StripPrefix": true,
                "Keys": [
                    {
                        "Name": "key-primary",
                        "Key": "sk-example-api-key-primary",
                        "Weight": 70
                    },
                    {
                        "Name": "key-secondary",
                        "Key": "sk-example-api-key-secondary",
                        "Weight": 30
                    }
                ],
                "KeyPolicy": {
                    "Strategy": "weighted_random",
                    "MaxRetries": 3,
                    "RetryBackoffInitial": 500,
                    "RetryBackoffMax": 5000,
                    "SessionAffinity": true,
                    "SessionAffinityTTL": 600,
                    "SessionAffinityRedisPrefix": "bfe:ai:key_affinity",
                    "SessionAffinityPenaltyEnable": true
                },
                "ModelMapping": {
                    "gpt-4": "backend-gpt-4-model"
                },
                "ModelTable": {
                    "Currency": "RMB",
                    "Models": [
                        {
                            "Provider": "deepseek",
                            "Model": "deepseek-v3",
                            "BaseModel": "deepseek-v3",
                            "Mode": "chat",
                            "Capabilities": ["chat", "reasoning", "tools"],
                            "SupportedParameters": ["temperature", "max_tokens"],
                            "Limits": {
                                "context_window": 128000,
                                "max_input_tokens": 128000,
                                "max_output_tokens": 8192
                            },
                            "Prices": {
                                "input_cost_per_token": 0.000002,
                                "output_cost_per_token": 0.000008
                            }
                        }
                    ]
                }
            }
        },
        "epp_cluster_example": {
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
                "BalanceMode": "EPP",
                "HashConf": {
                    "HashStrategy": 0,
                    "HashHeader": "Cookie:UID",
                    "SessionSticky": false
                },
                "EPPAddr": [
                    "10.0.0.1:9002",
                    "10.0.0.2:9002"
                ],
                "EPPCheck": {
                    "CheckInterval": "2s",
                    "FailThreshold": 3,
                    "Cooldown": "45s",
                    "SuccessThreshold": 2
                },
                "EPPTimeout": {
                    "Connect": "500ms",
                    "Call": "3s"
                },
                "EPPTLS": {
                    "Insecure": false,
                    "CAFile": "../conf/epp/epp_ca.crt"
                },
                "EPPBreaker": {
                    "WindowSize": 100,
                    "MinVolume": 20,
                    "ErrorRatePercent": 50,
                    "OpenTimeout": "30s"
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
