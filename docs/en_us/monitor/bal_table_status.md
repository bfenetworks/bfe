# Introduction

bal_table_status monitor table state for maintain backend cluster.

# Monitor Item

| Monitor Item | Description                                                  |
| ------------ | ------------------------------------------------------------ |
| Balancers    | State of cluster, it is map data, key is cluster name, value is cluster state |
| BackendNum   | Number of cluster backend                                    |

## cluster state

| Monitor Item | Description                                                  |
| ------------ | ------------------------------------------------------------ |
| SubClusters  | State of sub-cluster, it is map data, key is sub-cluster name, value is number of sub-cluster backend |
| BackendNum   | Number of sub-cluster backend                                |

