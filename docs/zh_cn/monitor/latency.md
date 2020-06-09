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

