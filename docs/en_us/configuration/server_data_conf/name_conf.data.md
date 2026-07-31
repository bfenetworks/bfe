# Naming Configuration

## Introduction

name_conf.data records the mapping between service name and service instances.

## Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of config file                                       |
| Config      | Object<br>Mapping between service name and instances                   |
| Config{k}   | String<br>Service name                                                 |
| Config{v}   | Object<br>A list of instances                                          |
| Config{v}[] | Object<br>Instance information                                 |
| Config{v}[].Host    | String<br>Instance address                                     |
| Config{v}[].Port    | Integer<br>Instance port                                       |
| Config{v}[].Weight  | Integer<br>Instance weight                                     |

**Note:** `name_conf.data` is optional. It is loaded only when `NameConf` is configured in the `[Server]` section of `bfe.conf`.

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
