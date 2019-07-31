# 简介

mod_trust_clientip 是模块trust_clientip的状态信息。

# 监控信息

| 监控项                       | 描述                                   |
| ---------------------------- | -------------------------------------- |
| CONN_ADDR_INTERNAL           | 来源于内部的连接数                     |
| CONN_ADDR_INTERNAL_NOT_TRUST | 来源于内部，但是不在信任列表中的请求数 |
| CONN_TOTAL                   | 所有请求数                             |
| CONN_TRUST_CLIENTIP          | 来源于信任地址的请求数                 |

