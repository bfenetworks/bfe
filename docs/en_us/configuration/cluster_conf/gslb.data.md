# SubClusters Balancing Configuration

## Introduction

gslb.data records the load balancing config between sub-clusters.

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Hostname | String | Hostname of gslb scheduler | Y | Identifies the host/system that generates this configuration file | Non-empty |
| Ts | String | Timestamp of config file | Y | Identifies the generation time of the configuration file | Non-empty |
| Clusters | Object | Load balancing weights among sub-clusters | Y | Key is cluster name, value is sub-cluster weight mapping | Non-empty |
| Clusters{k} | String | Cluster name | Y | Key of Clusters | Non-empty |
| Clusters{v} | Object | Weight config for each sub-cluster | Y | Key is sub-cluster name, value is weight | Non-empty |
| Clusters{v}{k} | String | Name of sub-cluster | Y | Key of Clusters{v}; reserved `GSLB_BLACKHOLE` represents the blackhole sub-cluster which discards all incoming requests, used for overload protection | Non-empty |
| Clusters{v}{v} | Integer | Weight of sub-cluster | Y | See [Weight](../00-common.md#8-weight) type definition | Type is [Weight](../00-common.md#8-weight); sum of positive weights across sub-clusters must be greater than 0 |

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
