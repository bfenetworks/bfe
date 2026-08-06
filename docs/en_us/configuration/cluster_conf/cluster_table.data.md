# Instances Balancing Configuration

## Introduction

cluster_table.data records the load balancing config among instances.

## Configuration

### Basic configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | See [Version](../00-common.md#5-version) type definition | Type is [Version](../00-common.md#5-version) |
| Config | Object | Config of all clusters | Y | Key is cluster name, value is sub-cluster config | Non-empty |
| Config{k} | String | Name of cluster | Y | Key of Config | Non-empty |
| Config{v} | Object | Config of cluster | Y | Key is sub-cluster name, value is instance list | Non-empty |
| Config{v}{k} | String | Name of subcluster | Y | Key of Config{v} | Non-empty |
| Config{v}{v} | []Object | Config of subcluster (a list of instances) | Y | Contains multiple instance configs | Non-empty; each sub-cluster must contain at least one instance with `Weight > 0` |

### Instance configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Addr | [Hostname](../00-common.md#6-hostname) | Listen address of instance | Y | See [Hostname](../00-common.md#6-hostname) type definition | Type is [Hostname](../00-common.md#6-hostname) |
| Port | [Port](../00-common.md#1-port) | Port of instance | Y | See [Port](../00-common.md#1-port) type definition | Type is [Port](../00-common.md#1-port) |
| Weight | [Weight](../00-common.md#8-weight) | Weight of instance | Y | See [Weight](../00-common.md#8-weight) type definition | Type is [Weight](../00-common.md#8-weight); must be >= 0 |
| Name | String | Name of instance | Y | Instance identifier | Non-empty |

**Note:** Each sub-cluster must contain at least one backend with `Weight > 0`.

## Example

```json
{
    "Config": {
        "cluster_example": {
            "example.bfe.bj": [
                {
                    "Addr": "10.199.189.26",
                    "Name": "example_hostname",
                    "Port": 10257,
                    "Weight": 10
                }
            ]
        }
    }, 
    "Version": "20190101000000"
}
```
