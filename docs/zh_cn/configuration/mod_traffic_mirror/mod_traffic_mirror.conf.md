# mod_traffic_mirror 基础配置

## 配置简介

`mod_traffic_mirror.conf` 是 `mod_traffic_mirror` 模块的基础配置文件，用于指定镜像规则文件路径、镜像子请求的分层超时、并发上限与熔断参数等。

`mod_traffic_mirror` 对符合规则的请求做**流量镜像（shadow traffic）**：主请求正常转发的同时，异步复制一份发送到镜像目标集群；镜像响应完整读空后丢弃（流式响应读到 SSE `[DONE]`），仅记录状态码、usage、finish_reason、错误分类与延迟等指标。镜像链路的任何失败都只计数，不影响主请求。

## 配置描述

| 配置项 | 类型 | 参数含义 | 必填 | 补充描述 | 合法性条件 |
| ------ | ---- | -------- | ---- | -------- | ---------- |
| Basic.ProductRulePath | String | 镜像规则文件路径 | Y | - | 文件须存在且可读 |
| Basic.ConnectTimeoutMs | Integer | 连接建立超时（毫秒） | N | 默认 2000 | 必须大于 0 |
| Basic.TTFBTimeoutMs | Integer | 首字节超时（毫秒，TTFT 兜底） | N | 默认 30000 | 必须大于 0 |
| Basic.TotalTimeoutMs | Integer | 镜像请求总时长上限（毫秒） | N | 默认 600000，匹配长推理；SSE 读空兜底 | 必须 ≥ TTFBTimeoutMs |
| Basic.MaxMirrorBodyBytes | Integer | 请求体镜像大小上限（字节） | N | 默认 2097152（2MB）；超限请求跳过镜像并计数 | (0, 8MB] |
| Basic.MaxResponseBodyBytes | Integer | 镜像响应最大读取字节 | N | 默认 16777216（16MB）；超出截断丢弃并计数 | 必须大于 0 |
| Basic.MaxConcurrent | Integer | 模块级镜像并发上限 | N | 默认 1024，即 worker 池大小 | 必须大于 0 |
| Basic.QueueCapacity | Integer | 提交队列容量 | N | 默认 4096；队列满时新任务直接丢弃并计数 | 非负整数 |
| Basic.CircuitBreakerFailThreshold | Integer | 触发熔断的连续失败次数 | N | 默认 50，按镜像目标 cluster 独立计数 | 必须大于 0 |
| Basic.CircuitBreakerCooldownSec | Integer | 熔断冷却窗口（秒） | N | 默认 30；冷却结束后放行探测请求 | 必须大于 0 |
| Log.OpenDebug | Boolean | 是否开启 debug 日志 | N | - | - |

> 说明：
> - 三个超时相互独立：连接超时由 dialer 强制执行；首字节超时通过定时器硬取消（未收到首字节即放弃）；总时长上限由请求 context 兜底，覆盖整个发送与响应读空过程。
> - 并发模型为固定 worker 池（`MaxConcurrent` 个 worker 消费有界队列）：提交永不阻塞主链路，突发流量在队列中缓冲，队列满即丢弃并计数，保证镜像风暴不会反压网关。
> - 镜像请求按目标 cluster 熔断：熔断期间该 cluster 的镜像任务快速丢弃并计数，不发起连接。

## 配置示例

```ini
[basic]
ProductRulePath = ../conf/mod_traffic_mirror/mirror_rule.data
ConnectTimeoutMs = 2000
TTFBTimeoutMs = 30000
TotalTimeoutMs = 600000
MaxMirrorBodyBytes = 2097152
MaxResponseBodyBytes = 16777216
MaxConcurrent = 1024
QueueCapacity = 4096
CircuitBreakerFailThreshold = 50
CircuitBreakerCooldownSec = 30

[log]
OpenDebug = false
```
