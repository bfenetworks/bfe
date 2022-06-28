# Balancing Details

## Introduction

The endpoint `/monitor/bal_table_status` exposes metrics about backend clusters.

## Metrics

| Metric       | Description                                                  |
| ------------ | ------------------------------------------------------------ |
| Balancers    | State of cluster, it is map data, key is cluster name, value is cluster state |
| BackendNum   | Number of cluster backend                                    |

### cluster state

| Monitor Item | Description                                                  |
| ------------ | ------------------------------------------------------------ |
| SubClusters  | State of sub-cluster, it is map data, key is sub-cluster name, value is number of sub-cluster backend |
| BackendNum   | Number of sub-cluster backend                                |
