# Latency Histogram

## Introduction

| Endpoint                              | Description       |
| ------------------------------------- | ----------------- |
| /monitor/proxy_handshake_delay        | Latency of the TLS handshake |
| /monitor/proxy_handshake_full_delay   | Latency of the TLS full handshake |
| /monitor/proxy_handshake_resume_delay | Latency of the TLS abbreviated handshake |
| /monitor/proxy_delay                  | Forwarding Latency for the GET requests |
| /monitor/proxy_post_delay             | Forwarding Latency for the POST requests |

## Metrics

| Metric       | Description                               |
| ------------ | ----------------------------------------- |
| Interval     | Statistical period (second)               |
| ProgramName  | Program name                              |
| KeyPrefix    | Key prefix                                |
| CurrTime     | Start time of current statistics          |
| Current      | Latency histogram for current statistics   |
| PastTime     | Start time of last statistics             |
| Past         | Latency histogram for last statistics      |

## Special Notes for Prometheus format

BFE can expose metrics in various formats.

Unlike other formats, in the Prometheus format latency histogram, counter for a bucket with lager upper bound will include the number of events in buckets with smaller upper bound.  See [Histogram](https://prometheus.io/docs/concepts/metric_types/#histogram) in Prometheus document for more detail.  

Example:

- proxy_handshake_delay_Past_bucket{le="1000"} is counter of handshakes with <= 1000 ms delay in last statistic interval
- proxy_handshake_delay_Past_bucket{le="2000"} is counter of handshakes with <= 2000 ms delay (includes those with <=1000 ms delay) in last statistic interval
- proxy_handshake_delay_Past_bucket{le="+Inf"} is counter of handshakes with less than infinity (equals total count) in last statistic interval. It is equal to proxy_handshake_delay_Past_count.
