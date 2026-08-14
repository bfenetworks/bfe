# mod_otel

## 模块简介

mod_otel 基于原生 OpenTelemetry SDK 实现，支持将 BFE 处理请求的过程以 OpenTelemetry Trace 的形式导出到 OTLP 兼容的后端（如 Jaeger、Tempo、OpenTelemetry Collector 等）。

与 mod_trace 不同，mod_otel 不依赖 OpenTracing 框架，直接使用 OpenTelemetry SDK，提供更现代的链路追踪能力。

## 基础配置

### 配置描述

模块配置文件: conf/mod_otel/mod_otel.conf

#### 基础配置项

| 配置项            | 描述                                  |
| ----------------- | ------------------------------------- |
| Basic.Enabled     | Boolean<br>是否启用 OpenTelemetry     |
| Basic.ServiceName | String<br>服务名，默认值为 bfe        |
| Basic.Endpoint    | String<br>OTLP gRPC 端点地址，默认值为 localhost:4317 |
| Basic.Insecure    | Boolean<br>是否使用不安全的 gRPC 连接 |
| Basic.SampleRate  | Float<br>采样率，取值范围为 0.0 到 1.0，默认值为 1.0 |

#### 日志配置项

| 配置项        | 描述                              |
| ------------- | --------------------------------- |
| Log.OpenDebug | Boolean<br>是否启用模块调试日志开关 |

### 配置示例

```ini
[Basic]
# 是否启用 OpenTelemetry (true/false)
Enabled = true

# 服务名
ServiceName = bfe

# OTLP gRPC 端点地址
Endpoint = localhost:4317

# 是否使用不安全连接
Insecure = true

# 采样率
SampleRate = 1.0

[Log]
# 是否启用调试日志
OpenDebug = false
```

## 收集的 Trace 属性

mod_otel 会在 span 中记录以下属性：

| 属性名            | 说明                       |
| ----------------- | -------------------------- |
| http.method       | HTTP 请求方法              |
| http.url          | HTTP 请求 URL              |
| http.host         | HTTP 请求 Host             |
| http.scheme       | HTTP 请求协议，http 或 https |
| user_agent        | User-Agent 请求头          |
| remote_addr       | 客户端地址                 |
| log_id            | BFE 日志 ID                |
| http.status_code  | HTTP 响应状态码            |
| product           | 产品线名称                 |
| cluster           | 后端集群名称               |
| subcluster        | 子集群名称                 |
| backend           | 后端实例地址               |

## Span 状态

根据 HTTP 响应状态码，mod_otel 会设置 span 的 Status：

- HTTP 状态码 < 400：Status 为 Ok
- HTTP 状态码 >= 400：Status 为 Error，并记录错误消息

## 上下文传播

mod_otel 支持通过 W3C TraceContext 标准在请求头中传播 trace 上下文，默认传播的请求头包括：

- traceparent
- tracestate
- baggage
