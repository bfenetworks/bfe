# 延时统计

## 简介

| 接口                                  | 描述              |
| ------------------------------------- | ----------------- |
| /monitor/proxy_handshake_delay        | TLS握手延时       |
| /monitor/proxy_handshake_full_delay   | TLS完全握手延时   |
| /monitor/proxy_handshake_resume_delay | TLS简化握手延时   |
| /monitor/proxy_delay                  | GET请求转发延时   |
| /monitor/proxy_post_delay             | POST请求转发延时  |

## 监控项

| 监控项      | 描述                   |
| ----------- | ---------------------- |
| Interval    | 统计周期               |
| ProgramName | 程序名称               |
| KeyPrefix   | 监控项名称前缀         |
| CurrTime    | 当前统计周期的起始时间 |
| Current     | 当前统计周期的延时统计 |
| PastTime    | 上个统计周期的起始时间 |
| Past        | 上个统计周期的延时统计 |

## Prometheus格式的特别说明

BFE可以暴露多种格式的监控指标。

与其他格式不同，Prometheus格式的延时统计中，有较高的上界的Bucket的计数值，会包含了有更小的上界的Bucket中的事件数。详见Prometheus官方文档中[Histogram](https://prometheus.io/docs/concepts/metric_types/#histogram)的相关说明。

示例:

- proxy_handshake_delay_Past_bucket{le="1000"} 是上一个统计周期中，延时小于等于 1000 ms 的握手的数量。
- proxy_handshake_delay_Past_bucket{le="2000"} 是上一个统计周期中，延时小于等于 2000 ms 的握手的数量 (包含延时小于等于 1000 ms 的握手数量)。
- proxy_handshake_delay_Past_bucket{le="+Inf"} 是上一个统计周期中，延时小于等于“无穷大” 的握手的数量（即是总的握手数量）这也等于proxy_handshake_delay_Past_count。
