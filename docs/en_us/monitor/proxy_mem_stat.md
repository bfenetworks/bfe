# Introduction

proxy_mem_stat monitor memory state of BFE server.

# Monitor Item

Memory State is descriped by MemStat in runtime package in golang.

| Monitor Item | Description                                                  |
| ------------ | ------------------------------------------------------------ |
| Alloc        | Bytes allocated and still in use                             |
| TotalAlloc   | Bytes allocated (even if freed)                              |
| Sys          | Bytes obtained from system (sum of XxxSys below)             |
| Lookups      | Number of pointer lookups                                    |
| Mallocs      | Number of mallocs                                            |
| Frees        | Number of frees                                              |
| HeapAlloc    | Bytes allocated and still in use                             |
| HeapSys      | Bytes obtained from system                                   |
| HeapIdle     | Bytes in idle spans                                          |
| HeapInuse    | Bytes in non-idle span                                       |
| HeapReleased | Bytes released to the OS                                     |
| HeapObjects  | Total number of allocated objects                            |
| StackInuse   | Bytes used now by stack allocator                            |
| StackSys     | Bytes obtained from system by stack allocator                |
| MSpanInuse   | Mspan structures used now                                    |
| MSpanSys     | Mspan structures obtained from system                        |
| MCacheInuse  | Mcache structures used now                                   |
| MCacheSys    | Mcache structures obtained from system                       |
| BuckHashSys  | Profiling bucket hash table                                  |
| GCSys        | GC metadata                                                  |
| OtherSys     | Other system allocations                                     |
| NextGC       | Next collection will happen when HeapAlloc ≥ this amount     |
| LastGC       | End time of last collection (nanoseconds since 1970)         |
| PauseTotalNs | Total time of GC pause time                                  |
| PauseNs      | Circular buffer of recent GC pause durations, most recent at [(NumGC+255)%256] |
| PauseEnd     | Circular buffer of recent GC pause end times                 |
| NumGC        | Number of GC                                                 |
| EnableGC     | Enable GC or not                                             |
| DebugGC      | Enbale debug GC or not                                       |
| BySize       | Per-size allocation statistics                               |