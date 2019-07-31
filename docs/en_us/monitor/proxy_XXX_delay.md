# Introduction

proxy_XXX_delay monitor state of proxy delay.

# Monitor Item

| Monitor Item | Description                               |
| ------------ | ----------------------------------------- |
| Interval     | Interval of get proxy delay.              |
| KeyPrefix    | Key prefix                                |
| ProgramName  | Program name                              |
| CurrTime     | Current time                              |
| Current      | Proxy delay data of current interval time |
| PastTime     | Last statistic time                       |
| Past         | Proxy delay data of last statistic time   |

## proxy Delay Data

| Monitor Item | Description                                                  |
| ------------ | ------------------------------------------------------------ |
| BucketSize   | Size of each delay bucket, e.g., 1(ms) or 2(ms)              |
| BucketNum    | Number of bucket                                             |
| Count        | Total number of samples                                      |
| Sum          | Summary data, in Microsecond                                 |
| Ave          | Average data, in Microsecond                                 |
| Counters     | Counters are counters for each bucket. e.g., for bucketSize == 1ms, BucketNum == 5, counters are for 0-1, 1-2, 2-3, 3-4, 4-5, >5 |

