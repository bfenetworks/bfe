# Introduction 

Enable trace for requests based on defined rules.

# Module configuration

## Description
 conf/mod_trace/mod_trace.conf
 
### Basic configuration

| Config Item                   | Description                     |
| ------------------------------| --------------------------------|
| Basic.DataPath                | String<br>path of rule configuraiton |
| Basic.ServiceName             | String<br>service name |
| Basic.TraceAgent              | String<br>which trace agent to use (jaeger/zipkin) | 
| Log.OpenDebug                 | Boolean<br>debug flag of module |

### Configuration about Zipkin

| Config Item                   | Description                     |
| ------------------------------| --------------------------------|
| Zipkin.HTTPEndpoint           | String<br>http endpoint to report traces to |
| Zipkin.SameSpan               | String<br>whether to use Zipkin SameSpan RPC style traces |
| Zipkin.ID128Bit               | String<br>whether to use 128 bit root span IDs |
| Zipkin.SampleRate             | Float<br>the rate between 0.0001 and 1.0 of requests to trace |

### Configuration about Jaeger

| Config Item                   | Description                     |
| ------------------------------| --------------------------------|
| Jaeger.SamplingServerURL      | String<br>the address of jaeger-agent's HTTP sampling server |
| Jaeger.SamplingType           | String<br>the type of the sampler: const, probabilistic, rateLimiting, or remote |
| Jaeger.SamplingParam          | Float<br>param passed to the sampler |
| Jaeger.LocalAgentHostPort     | String<br>the address of jaeger-agent which recives spans |
| Jaeger.Propagation            | String<br>which propagation format to use (jaeger/b3) |
| Jaeger.Gen128Bit              | Boolean<br>whether to use 128 bit root span IDs |
| Jaeger.TraceContextHeaderName | String<br>the http header name used to propagate tracing context |
| Jaeger.CollectorEndpoint      | String<br>the address of jaeger-collector |
| Jaeger.CollectorUser          | String<br>basic http authentication when sending spans to jaeger-collector |
| Jaeger.CollectorPassword      | String<br>basic http authentication when sending spans to jaeger-collector |

## Example

### Example for Zipkin
```
[Basic]
DataPath = mod_trace/trace_rule.data
ServiceName = bfe

# Which trace agent to use (zipkin, jaeger)
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

```
[Basic]
DataPath = mod_trace/trace_rule.data
ServiceName = bfe

# Which trace agent to use (zipkin, jaeger)
TraceAgent = zipkin

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

# Rule configuration

## Description
conf/mod_trace/trace_rule.data

| Config Item                | Description                                  |
| -------------------------- | -------------------------------------------- |
| Version                    | String<br>Verson of the config file          |
| Config                     | Object<br>Trace rules for each product       |
| Config[k]                  | String<br>Product name                       |
| Config[v]                  | Object<br>A list of trace rules              |
| Config[v][]                | Object<br>A trace rule                       |
| Config[v][].Cond           | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config[v][].Enable         | Boolean<br>Enable trace                       |
  
## Example
```
{
  "Version": "20200218210000",
  "Config": {
    "example_product": [
       {
         "Cond": "req_host_in(\"example.org\")",
         "Enable": true
       }
    ]
  }
}
```
