# mod_otel

## Introduction

mod_otel is based on the native OpenTelemetry SDK. It supports exporting BFE request processing as OpenTelemetry Trace data to OTLP-compatible backends such as Jaeger, Tempo, and OpenTelemetry Collector.

Unlike mod_trace, mod_otel does not depend on the OpenTracing framework. It directly uses the OpenTelemetry SDK to provide modern distributed tracing capabilities.

## Module Configuration

### Description

Module config file: conf/mod_otel/mod_otel.conf

#### Basic Configuration

| Config Item       | Description                                  |
| ----------------- | -------------------------------------------- |
| Basic.Enabled     | Boolean<br>Whether to enable OpenTelemetry   |
| Basic.ServiceName | String<br>Service name, default is bfe       |
| Basic.Endpoint    | String<br>OTLP gRPC endpoint, default is localhost:4317 |
| Basic.Insecure    | Boolean<br>Whether to use insecure gRPC connection |
| Basic.SampleRate  | Float<br>Sampling rate, range from 0.0 to 1.0, default is 1.0 |

#### Log Configuration

| Config Item   | Description                     |
| ------------- | --------------------------------|
| Log.OpenDebug | Boolean<br>Debug flag of module |

### Example

```ini
[Basic]
# Whether to enable OpenTelemetry (true/false)
Enabled = true

# Service name
ServiceName = bfe

# OTLP gRPC endpoint
Endpoint = localhost:4317

# Whether to use insecure connection
Insecure = true

# Sampling rate
SampleRate = 1.0

[Log]
# Whether to enable debug logging
OpenDebug = false
```

## Collected Trace Attributes

mod_otel records the following attributes in each span:

| Attribute Name    | Description                |
| ----------------- | -------------------------- |
| http.method       | HTTP request method        |
| http.url          | HTTP request URL           |
| http.host         | HTTP request Host          |
| http.scheme       | HTTP request scheme, http or https |
| user_agent        | User-Agent request header  |
| remote_addr       | Client address             |
| log_id            | BFE log ID                 |
| http.status_code  | HTTP response status code  |
| product           | Product name               |
| cluster           | Backend cluster name       |
| subcluster        | Subcluster name            |
| backend           | Backend instance address   |

## Span Status

mod_otel sets the span Status based on the HTTP response status code:

- HTTP status code < 400: Status is Ok
- HTTP status code >= 400: Status is Error, with an error message

## Context Propagation

mod_otel supports propagating trace context through request headers based on the W3C TraceContext standard, including:

- traceparent
- tracestate
- baggage
