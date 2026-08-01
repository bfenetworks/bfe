# mod_tcp_keepalive

## 模块简介

mod_tcp_keepalive管理TCP长连接心跳包的发送策略。

在某些场景下可能对耗电量十分敏感，比如智能手表待机状态，希望能够停止发送TCP长连接时的心跳包，或者降低其发送频率，此模块即可以用来处理此类或者其他需要管理TCP心跳包发送策略的场景。

## 基础配置

模块基础配置文件说明详见 [mod_tcp_keepalive.conf](../../configuration/mod_tcp_keepalive/mod_tcp_keepalive.conf.md)。

## 规则配置

模块规则配置文件说明详见 [tcp_keepalive.data](../../configuration/mod_tcp_keepalive/tcp_keepalive.data.md)。

## 监控项

| 监控项        | 描述                         |
| ------------- | ---------------------------- |
| CONN_TO_SET    | 命中配置规则的连接总数                     |
| CONN_SET_KEEP_IDLE | 设置keepIdle属性的连接数 |
| CONN_SET_KEEP_IDLE_ERROR | 设置keepIdle属性失败的连接数 |
| CONN_SET_KEEP_INTVL | 设置keepIntvl属性的连接数 |
| CONN_SET_KEEP_INTVL_ERROR | 设置keepIntvl属性失败的连接数 |
| CONN_SET_KEEP_CNT | 设置keepCnt属性的连接数 |
| CONN_SET_KEEP_CNT_ERROR | 设置keepCnt属性失败的连接数 |
| CONN_DISABLE_KEEP_ALIVE | 设置disable属性的连接数 |
| CONN_DISABLE_KEEP_ALIVE_ERROR | 设置disable属性失败的连接数 |
| CONN_CONVERT_TO_TCP_CONN_ERROR | 将连接类型转换为TCPConn类型失败的次数 |
