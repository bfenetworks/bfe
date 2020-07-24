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
| Current      | Latency histgram for current statistics   |
| PastTime     | Start time of last statistics             |
| Past         | Latency histgram for last statistics      |
