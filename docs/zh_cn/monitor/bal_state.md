# Balance State

## 简介

`/monitor/bal_state`接口返回子集群级别负载均衡指标。

## 监控项

| 监控项                       | 描述                           |
| --------------------------- | ----------------------------- |
| ERR_BK_NO_BACKEND           | 未找到后端的错误数               |
| ERR_BK_NO_SUB_CLUSTER       | 未找到子集群的错误数              |
| ERR_BK_NO_SUB_CLUSTER_CROSS | 跨子集群转发时，未找到子集群的错误数 |
| ERR_BK_RETRY_TOO_MANY       | 转发请求达到最大重试次数的错误数    |
| ERR_GSLB_BLACKHOLE          | 转发到黑洞（被丢弃）的请求数        |
