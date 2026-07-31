# SubClusters Balancing Configuration

## Introduction

gslb.data records the load balancing config between sub-clusters.

## Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Clusters    | Object<br>cluster config |
| Clusters{k} | String<br>cluster name |
| Clusters{v} | Object<br>weight config for each sub-cluster        |
| Clusters{v}{k} | String<br>name of sub-cluster<br>GSLB_BLACKHOLE is the name of blackhole sub-cluster which discards all incoming requests |
| Clusters{v}{v} | Integer<br>weight of sub-cluster<br>The sum of positive weights must be greater than 0; BFE distributes traffic proportionally. The weights are not required to be in [0, 100] and are not required to sum to 100 |
| Hostname    | String<br>Hostname of gslb scheduler (required)                                   |
| Ts          | String<br>Timestamp of config file (required)                                     |

## Example

```json
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
