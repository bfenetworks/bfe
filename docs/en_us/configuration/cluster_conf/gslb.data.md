# Introduction

gslb.data records the load balancing config between sub-clusters. 

# Configuration

| Config Item | Type   | Description                                                  |
| ----------- | ------ | ------------------------------------------------------------ |
| Clusters    | Struct | Key: cluster name. Value: weight for each sub-cluster        |
| Hostname    | String | Hostname of gslb scheduler                                   |
| Ts          | String | Timestamp of config file                                     |

# Example

```
{
    "Clusters": {
        "cluster_example": {
            "GSLB_BLACKHOLE": 0,
            "example.bfe.bj": 100
        }
    },
    "Hostname": "gslb-sch.example.com",
    "Ts": "20190101000000"
}
```


