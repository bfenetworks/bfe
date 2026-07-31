# Instances Balancing Configuration

## Introduction

cluster_table.data records the load balancing config among instances.

## Configuration

### Basic configuration

| Config Item           | Description                     |
| --------------------- | ------------------------------- |
| Version               | String<br>Version of config file |
| Config                | Object<br>config of all clusters |
| Config{k}             | String<br>name of cluster |
| Config{v}             | Object<br>config of cluster |
| Config{v}{k}          | String<br>name of subcluster |
| Config{v}{v}          | Object<br>config of subcluster(a list of instance) |

### Instance configuration

| Config Item           | Description                     |
| --------------------- | ------------------------------- |
| Addr                  | String<br>listen address of instance (required) |
| Name                  | String<br>name of instance (required) |
| Port                  | Integer<br>port of instance (required) |
| Weight                | Integer<br>weight of instance (required) |

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
