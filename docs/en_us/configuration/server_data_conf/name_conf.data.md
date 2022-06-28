# Naming Configurationn

## Introduction

name_conf.data records the mapping between service name and service instances.

## Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of config file                                       |
| Config      | Struct<br>Mapping between service name and instances                   |
| Config{k}   | String<br>Service name                                                 |
| Config{v}   | Struct<br>A list of instances                                          |
| Config{v}[].Host    | String<br>Instance address                                     |
| Config{v}[].Port    | Integer<br>Instance port                                       |
| Config{v}[].Weight  | Integer<br>Instance weight                                     |

## Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "example.redis.cluster": [
            {
                "Host": "192.168.1.1",
                "Port": 6439,
                "Weight": 10
            }
        ]
    }
}
```
