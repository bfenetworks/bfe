# 简介

proxy_mem_stat 是BFE的内存的状态信息

# 监控项

| 监控项       | 描述                                                     |
| ------------ | -------------------------------------------------------- |
| Alloc        | 已分配且仍在使用的字节数                                 |
| TotalAlloc   | 已分配（包括已释放的）字节数                             |
| Sys          | 从系统中获取的字节数（应当为下面 XxxSys 之和）           |
| Lookups      | 指针查找数                                               |
| Mallocs      | malloc 数                                                |
| Frees        | free 数                                                  |
| HeapAlloc    | 已分配且仍在使用的字节数                                 |
| HeapSys      | 从系统中获取的字节数                                     |
| HeapIdle     | 空闲区间的字节数                                         |
| HeapInuse    | 非空闲区间的字节数                                       |
| HeapReleased | 释放给OS的字节数                                         |
| HeapObjects  | 已分配对象的总数                                         |
| StackInuse   | 栈分配器使用的字节（正在使用的字节数）                   |
| StackSys     | 栈分配器使用的字节（从系统获取的字节数）                 |
| MSpanInuse   | mspan（内存区间）结构数（正在使用的字节数）              |
| MSpanSys     | mspan（内存区间）结构数（从系统获取的字节数）            |
| MCacheInuse  | mcache（内存缓存）结构数（正在使用的字节数）             |
| MCacheSys    | mcache（内存缓存）结构数（从系统获取的字节数）           |
| BuckHashSys  | 分析桶散列表                                             |
| GCSys        | GC 元数据                                                |
| OtherSys     | 其它系统分配                                             |
| NextGC       | 当HeapAlloc大于等于配置值时，下次GC收集将会进行          |
| LastGC       | 上次GC收集结束时间                                       |
| PauseTotalNs | GC暂停的总时间                                           |
| PauseNs      | 最近GC暂停时间的循环缓存，最近一次应为 [(NumGC+255)%256] |
| PauseEnd     | 最近GC暂停时间的循环缓存，最近一次的结束时间             |
| NumGC        | GC的数量                                                 |
| EnableGC     | 是否开启GC                                               |
| DebugGC      | 是否开启GC调试日志                                       |
| BySize       | 每个分配的大小统计。                                     |