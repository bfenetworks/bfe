# Balance Table Status

## 简介

`/monitor/bal_table_status`接口返回端集群的状态指标。

## 监控项

| 监控项      | 描述              |
| ---------- | ---------------- |
| Balancers  | 各集群的状态信息    |
| BackendNum | 所有集群后端实例总数 |

### 集群状态信息

| 监控项       | 描述                |
| ----------- | ------------------ |
| SubClusters | 各子集群的状态信息    |
| BackendNum  | 所有子集群后端实例总数 |
