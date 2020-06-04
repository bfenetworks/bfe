# Latency

## Introduction

| Endpoint                              | Description       | 
| ------------------------------------- | ----------------- |
| /monitor/proxy_delay                  | Latency of the forwarding GET requests |
| /monitor/proxy_post_delay             | Latency of the forwarding POST requests |
| /monitor/proxy_handshake_delay        | Latency of the TLS handshake |
| /monitor/proxy_handshake_full_delay   | Latency of the TLS full handshake |
| /monitor/proxy_handshake_resume_delay | Latency of the TLS abbreviated handshake |

## Metrics

| Metric       | Description                               |
| ------------ | ----------------------------------------- |
| Interval     | Interval of get proxy delay.              |
| KeyPrefix    | Key prefix                                |
| ProgramName  | Program name                              |
| CurrTime     | Current time                              |
| Current      | Latency histgram for current time slot    |
| PastTime     | Last statistic time                       |
| Past         | Latency histgram data of last time slot   |

## Lentency Histgram

| Monitor Item | Description                                                  |
| ------------ | ------------------------------------------------------------ |
| BucketSize   | Size of each delay bucket, e.g., 1(ms) or 2(ms)              |
| BucketNum    | Number of bucket                                             |
| Count        | Total number of samples                                      |
| Sum          | Summary data, in Microsecond                                 |
| Ave          | Average data, in Microsecond                                 |
| Counters     | Counters are counters for each bucket. e.g., for bucketSize == 1ms, BucketNum == 5, counters are for 0-1, 1-2, 2-3, 3-4, 4-5, >5 |

