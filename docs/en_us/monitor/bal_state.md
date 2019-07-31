# Introduction

bal_state monitor the subcluster level load balance state using gslb.

# Monitor Item

| Monitor Item                | Description                            |
| --------------------------- | -------------------------------------- |
| ERR_BK_NO_BACKEND           | Counter for no backend found           |
| ERR_BK_NO_SUB_CLUSTER       | Counter for no sub-cluster found       |
| ERR_BK_NO_SUB_CLUSTER_CROSS | Counter for no cross sub-cluster found |
| ERR_BK_RETRY_TOO_MANY       | Counter for reaching retry max times   |
| ERR_GSLB_BLACKHOLE          | Counter for denying by blackhole       |

