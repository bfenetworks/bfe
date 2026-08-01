# mod_trace 基础配置

## 配置简介

`mod_trace.conf` 是 `mod_trace` 模块的基础配置文件，用于指定规则配置文件路径、服务名及 trace 组件等。

## 配置描述

### 基础配置项

| 配置项            | 类型    | 参数含义            | 必填 | 补充描述                              | 合法性条件                                                   |
| ----------------- | ------- | ------------------- | ---- | ------------------------------------- | ------------------------------------------------------------ |
| Basic.DataPath    | String  | 规则配置文件路径    | Y    | 默认值为 `mod_trace/trace_rule.data`  | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Basic.ServiceName | String  | 服务名              | Y    | 用于标识当前服务，会传递给 trace 后端 | -                                                            |
| Basic.TraceAgent  | String  | trace 组件类型      | Y    | 可选值：`zipkin`、`jaeger`、`elastic` | 须为 `zipkin`、`jaeger`、`elastic` 之一                    |
| Log.OpenDebug     | Boolean | 是否开启 debug 日志 | N    | 默认值为 `False`                      | -                                                            |

### Zipkin 配置项

以下配置仅在 `Basic.TraceAgent` 为 `zipkin` 时生效。

| 配置项              | 类型    | 参数含义                        | 必填 | 补充描述         | 合法性条件           |
| ------------------- | ------- | ------------------------------- | ---- | ---------------- | -------------------- |
| Zipkin.HTTPEndpoint | String  | 接收 trace 信息的 HTTP 接口     | N    | 建议显式配置     | -                    |
| Zipkin.SameSpan     | Boolean | 客户端与服务端是否使用相同 span | N    | -                | -                    |
| Zipkin.ID128Bit     | Boolean | 是否使用 128 位 span ID         | N    | -                | -                    |
| Zipkin.SampleRate   | Float   | 请求抽样比例                    | N    | -                | 取值范围为 0.0 - 1.0 |

### Jaeger 配置项

以下配置仅在 `Basic.TraceAgent` 为 `jaeger` 时生效。

| 配置项                        | 类型    | 参数含义                                | 必填 | 补充描述     | 合法性条件                                                   |
| ----------------------------- | ------- | --------------------------------------- | ---- | ------------ | ------------------------------------------------------------ |
| Jaeger.SamplingServerURL      | String  | jaeger-agent 抽样服务地址               | N    | 建议显式配置 | -                                                            |
| Jaeger.SamplingType           | String  | 抽样类型                                | N    | 建议显式配置 | 须为 `const`、`probabilistic`、`rateLimiting`、`remote` 之一 |
| Jaeger.SamplingParam          | Float   | 抽样参数                                | N    | -            | 依 `SamplingType` 语义而定                                   |
| Jaeger.LocalAgentHostPort     | String  | 接收 span 信息的 jaeger-agent 地址      | N    | 建议显式配置 | -                                                            |
| Jaeger.Propagation            | String  | 透传格式                                | N    | -            | 须为 `jaeger` 或 `b3`                                        |
| Jaeger.Gen128Bit              | Boolean | 是否使用 128 位 span ID                 | N    | -            | -                                                            |
| Jaeger.TraceContextHeaderName | String  | 上下文中传递 trace 上下文的 header 名称 | N    | -            | -                                                            |
| Jaeger.CollectorEndpoint      | String  | jaeger-collector 地址                   | N    | -            | -                                                            |
| Jaeger.CollectorUser          | String  | jaeger-collector 认证用户名             | N    | -            | -                                                            |
| Jaeger.CollectorPassword      | String  | jaeger-collector 认证密码               | N    | -            | -                                                            |

### Elastic 配置项

以下配置仅在 `Basic.TraceAgent` 为 `elastic` 时生效。

| 配置项              | 类型    | 参数含义                      | 必填 | 补充描述     | 合法性条件 |
| ------------------- | ------- | ----------------------------- | ---- | ------------ | ---------- |
| Elastic.ServerURL   | String  | Elastic APM server 地址       | N    | 建议显式配置 | -          |
| Elastic.SecretToken | String  | Elastic APM server 认证 token | N    | -            | -          |

## 配置示例

### 基于 Zipkin

```ini
[Basic]
DataPath = mod_trace/trace_rule.data
ServiceName = bfe

# Which trace agent to use (zipkin, jaeger, elastic)
TraceAgent = zipkin

[Log]
OpenDebug = false

[Zipkin]
# Zipkin, only useful when the TraceAgent is zipkin

# HTTP Endpoint to report traces to
HTTPEndpoint = http://127.0.0.1:9411/api/v2/spans

# Use Zipkin SameSpan RPC style traces
SameSpan = false

# Use Zipkin 128 bit root span IDs
ID128Bit = true

# The rate between 0.0001 and 1.0 of requests to trace
SampleRate = 1.0
```

### 基于 Jaeger

```ini
[Basic]
DataPath = mod_trace/trace_rule.data
ServiceName = bfe

# Which trace agent to use (zipkin, jaeger, elastic)
TraceAgent = jaeger

[Log]
OpenDebug = false

[Jaeger]
# Jaeger, only useful when the TraceAgent is jaeger

# SamplingServerURL is the address of jaeger-agent's HTTP sampling server
SamplingServerURL = http://127.0.0.1:5778/sampling

# Type specifies the type of the sampler: const, probabilistic, rateLimiting, or remote
SamplingType = const

# Param is a value passed to the sampler.
# Valid values for Param field are:
# - for "const" sampler, 0 or 1 for always false/true respectively
# - for "probabilistic" sampler, a probability between 0 and 1
# - for "rateLimiting" sampler, the number of spans per second
# - for "remote" sampler, param is the same as for "probabilistic"
#   and indicates the initial sampling rate before the actual one
#   is received from the mothership.
SamplingParam = 1.0

# LocalAgentHostPort instructs reporter to send spans to jaeger-agent at this address
LocalAgentHostPort = 127.0.0.1:6831

# Which propagation format to use (jaeger/b3)
Propagation = jaeger

# Use Jaeger 128 bit root span IDs
Gen128Bit = true

# TraceContextHeaderName is the http header name used to propagate tracing context.
TraceContextHeaderName = uber-trace-id

# Instructs reporter to send spans to jaeger-collector at this URL
CollectorEndpoint = ""

# CollectorUser for basic http authentication when sending spans to jaeger-collector
CollectorUser = ""

# CollectorPassword for basic http authentication when sending spans to jaeger-collector
CollectorPassword = ""
```

### 基于 Elastic

```ini
[Basic]
DataPath = mod_trace/trace_rule.data
ServiceName = bfe

# Which trace agent to use (zipkin, jaeger, elastic)
TraceAgent = elastic

[Log]
OpenDebug = false

[Elastic]
# Elastic, only useful when TraceAgent is elastic

# Set the URL of the Elastic APM server
ServerURL = http://127.0.0.1:8200

# Set the token used to connect to Elastic APM Server
SecretToken = ""
```
