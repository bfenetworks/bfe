# Introduction

name_conf.data records the mapping between service name and service instances. 

# Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of config file                                       |
| Config      | Struct<br>Mapping between name and instances. Key: service name. Value:  a list of instances. Instance:<br>- Host: instance address <br>- Port: instance port<br>- Weight: instance weight |

# Example

```
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
