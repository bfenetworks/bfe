# 简介

proxy_XXX_delay 是转发延时的状态信息。

# 监控项

| 监控项      | 描述                         |
| ----------- | ---------------------------- |
| Interval    | 获取转发延时的时间间隔       |
| KeyPrefix   | key 前缀                     |
| ProgramName | 服务名称                     |
| CurrTime    | 当前时间                     |
| Current     | 当前采集时间间隔中的转发延时 |
| PastTime    | 上个采集时间                 |
| Past        | 上个采集时间的转发延时       |

## 转发延时详细信息

| 监控项     | 描述                                                         |
| ---------- | ------------------------------------------------------------ |
| BucketSize | 每个延时bucket的大小，例如：1(ms) 或 2(ms)                   |
| BucketNum  | bucket个数                                                   |
| Count      | 样例总数                                                     |
| Sum        | 汇总数据，单位为微秒                                         |
| Ave        | 平均数据，单位为微秒                                         |
| Counters   | 每个bucket的具体信息。例如：bucketSize == 1ms， BucketNum == 5, counters 为0-1, 1-2, 2-3, 3-4, 4-5, >5 |

