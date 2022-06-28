# mod_trace

## 模块简介

mod_trace根据自定义的条件，为请求开启分布式跟踪。

## 基础配置

### 配置描述

模块配置文件: conf/mod_trace/mod_trace.conf

#### 基础配置项

| 配置项                         | 描述                     |
| ------------------------------| -------------------------|
| Basic.DataPath                | String<br>规则配置文件路径 |
| Basic.ServiceName             | String<br>服务名 |
| Basic.TraceAgent              | String<br>设置trace组件，可选值：jaeger和zipkin |
| Log.OpenDebug                 | Boolean<br>是否启用模块调试日志开关 |

#### Zipkin配置项

| 配置项                         | 描述                     |
| ------------------------------| -------------------------|
| Zipkin.HTTPEndpoint           | String<br>设置接收trace信息的接口 |
| Zipkin.SameSpan               | String<br>客户端与服务端是否使用相同的span |
| Zipkin.ID128Bit               | String<br>是否使用128位span ID |
| Zipkin.SampleRate             | Float<br>设置请求抽样比例 |

#### Jaeger配置项

| 配置项                         | 描述                     |
| ------------------------------| -------------------------|
| Jaeger.SamplingServerURL      | String<br>设置抽样服务地址 |
| Jaeger.SamplingType           | String<br>设置抽样类型，可选值：const, probabilistic, rateLimiting, remote |
| Jaeger.SamplingParam          | Float<br>设置抽样参数 |
| Jaeger.LocalAgentHostPort     | String<br>设置接收span信息的jaeger-agent地址 |
| Jaeger.Propagation            | String<br>设置透传格式，可选值：jaeger或b3 |
| Jaeger.Gen128Bit              | Boolean<br>是否使用128位span ID |
| Jaeger.TraceContextHeaderName | String<br>设置上下文中header名称 |
| Jaeger.CollectorEndpoint      | String<br>设置jaeger-collector地址 |
| Jaeger.CollectorUser          | String<br>设置jaeger-collector认证用户名 |
| Jaeger.CollectorPassword      | String<br>设置jaeger-collector认证密码 |

#### Elastic配置项

| 配置项                         | 描述                                      |
| ------------------------------| -----------------------------------------|
| Elastic.ServerURL             | String<br>设置Elastic APM server          |
| Elastic.SecretToken           | String<br>设置Elastic APM server认证token |

### 配置示例

#### 基于Zipkin示例

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

#### 基于Jaeger示例

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

#### 基于Elastic示例

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

## 规则配置

### 配置描述

规则配置文件: conf/mod_trace/trace_rule.data

| 配置项                      | 描述                                         |
| -------------------------- | -------------------------------------------- |
| Version                    | String<br>配置文件版本                       |
| Config                     | Object<br>各产品线的规则列表                 |
| Config[k]                  | String<br>产品线名称                         |
| Config[v]                  | Object<br>产品线的规则列表                   |
| Config[v][]                | Object<br>产品线的规则                       |
| Config[v][].Cond           | String<br>规则的匹配条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config[v][].Enable         | Boolean<br>是否开启trace                      |
  
### 配置示例

```json
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
