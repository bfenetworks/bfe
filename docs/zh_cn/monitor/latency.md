# 延时统计

## 简介

| 接口                                  | 描述              |
| ------------------------------------- | ----------------- |
| /monitor/proxy_delay                  | 统计转发延时        |
| /monitor/proxy_post_delay             | 统计POST请求转发延时 |
| /monitor/proxy_handshake_delay        | 统计TLS握手延时     |
| /monitor/proxy_handshake_full_delay   | 统计TLS完全握手延时  |
| /monitor/proxy_handshake_resume_delay | 统计TLS简化握手延时  |

## 监控项

| 监控项       | 描述                  |
| ----------- | -------------------- |
| Interval    | 统计周期的时间间隔      |
| KeyPrefix   | format=kv或prometheus情况下，统计项前缀 |
| ProgramName | 程序名称              |
| CurrTime    | 当前统计周期起始时间    |
| Current     | 当前统计周期内的延时统计 |
| PastTime    | 上个统计周期起始时间    |
| Past        | 上个统计周期内的延时统计 |

### 延时统计详细信息

| 监控项      | 描述                              |
| ---------- | -------------------------------- |
| BucketSize | 每个延时bucket的大小，例如：1（毫秒） |
| BucketNum  | bucket个数                        |
| Count      | 请求总数                           |
| Sum        | 统计周期内总延迟，单位为微秒          |
| Ave        | 统计周期内平均延迟，单位为微秒         |
| Counters   | 统计延迟分布情况<br>例如：BucketSize=1，BucketNum=3，则统计延迟在0~1ms、1~2ms、2~3ms、>3ms的分布情况 |

