# Instances Balancing Configuration

## Introduction

cluster_table.data records the load balancing config among instances.

## Configuration

### Basic configuration

| Config Item           | Description                     |
| --------------------- | ------------------------------- |
| Version               | String<br>Verson of config file |
| Config                | Object<br>config of all clusters |
| Config{k}             | String<br>name of cluster |
| Config{v}             | Object<br>config of cluster |
| Config{v}{k}          | String<br>name of subcluster |
| Config{v}{v}          | Object<br>config of subcluster(a list of instance) |

### Instance configuration

| Config Item           | Description                     |
| --------------------- | ------------------------------- |
| Addr                  | String<br>ip address of instance |
| Name                  | String<br>name of instance |
| Port                  | String<br>port of instance |
| Weight                | String<br>weight of instance |

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
