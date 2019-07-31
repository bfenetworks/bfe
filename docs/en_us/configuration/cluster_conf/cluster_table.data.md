# Introduction

cluster_table.data records the load balancing config among instances.

# Configuration

| Config Item | Type   | Description                                                  |
| ----------- | ------ | ------------------------------------------------------------ |
| Version     | String | Verson of config file                                        |
| Config      | Struct | Instance config of sub-cluster in cluster. <br>cluster => sub-cluster => instance address and wight |

# Example

```
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
