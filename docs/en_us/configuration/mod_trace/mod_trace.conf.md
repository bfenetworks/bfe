# mod_trace Basic Configuration

## Configuration Introduction

`mod_trace.conf` is the basic configuration file for the `mod_trace` module, used to specify the rule configuration file path, service name, and trace agent.

## Configuration Description

### Basic Configuration Items

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration | Y | Default value is `mod_trace/trace_rule.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Basic.ServiceName | String | Service name | Y | Used to identify the current service and passed to the trace backend | - |
| Basic.TraceAgent | String | Which trace agent to use | Y | Optional values: `zipkin`, `jaeger`, `elastic` | Must be one of `zipkin`, `jaeger`, `elastic` |
| Log.OpenDebug | Boolean | Debug flag of module | N | Default value is `false` | - |

### Zipkin Configuration Items

The following configurations only take effect when `Basic.TraceAgent` is `zipkin`.

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Zipkin.HTTPEndpoint | String | HTTP endpoint to report traces to | N | Explicit configuration is recommended | - |
| Zipkin.SameSpan | Boolean | Whether to use Zipkin SameSpan RPC style traces | N | - | - |
| Zipkin.ID128Bit | Boolean | Whether to use 128 bit root span IDs | N | - | - |
| Zipkin.SampleRate | Float | The rate of requests to trace | N | - | Value range is 0.0 - 1.0 |

### Jaeger Configuration Items

The following configurations only take effect when `Basic.TraceAgent` is `jaeger`.

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Jaeger.SamplingServerURL | String | The address of jaeger-agent's HTTP sampling server | N | Explicit configuration is recommended | - |
| Jaeger.SamplingType | String | The type of the sampler | N | Explicit configuration is recommended | Must be one of `const`, `probabilistic`, `rateLimiting`, `remote` |
| Jaeger.SamplingParam | Float | Param passed to the sampler | N | - | Depends on the semantics of `SamplingType` |
| Jaeger.LocalAgentHostPort | String | The address of jaeger-agent which receives spans | N | Explicit configuration is recommended | - |
| Jaeger.Propagation | String | Which propagation format to use | N | - | Must be one of `jaeger`, `b3` |
| Jaeger.Gen128Bit | Boolean | Whether to use 128 bit root span IDs | N | - | - |
| Jaeger.TraceContextHeaderName | String | The HTTP header name used to propagate tracing context | N | - | - |
| Jaeger.CollectorEndpoint | String | The address of jaeger-collector | N | - | - |
| Jaeger.CollectorUser | String | Basic HTTP authentication user when sending spans to jaeger-collector | N | - | - |
| Jaeger.CollectorPassword | String | Basic HTTP authentication password when sending spans to jaeger-collector | N | - | - |

### Elastic Configuration Items

The following configurations only take effect when `Basic.TraceAgent` is `elastic`.

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Elastic.ServerURL | String | The URL of the Elastic APM server | N | Explicit configuration is recommended | - |
| Elastic.SecretToken | String | The token used to connect to Elastic APM Server | N | - | - |

## Configuration Example

### Example for Zipkin

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

### Example for Jaeger

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

### Example for Elastic

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
