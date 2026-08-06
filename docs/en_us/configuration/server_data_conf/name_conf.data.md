# Naming Configuration

## Introduction

name_conf.data records the mapping between service name and service instances.

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | See [Version](../00-common.md#5-version) type definition | Type is [Version](../00-common.md#5-version) |
| Config | Object | Mapping between service name and instances | Y | Key is service name, value is the instance information list | Non-empty |
| Config{k} | String | Service name | Y | Key of Config | Non-empty |
| Config{v} | []Object | Instance information list | Y | All instances corresponding to the service name | Non-empty |
| Config{v}[] | Object | Instance information | Y | Contains Host, Port and Weight | Non-empty |
| Config{v}[].Host | String | Instance address | Y | See [IPAddr](../00-common.md#7-ipaddr) type definition | Type is [IPAddr](../00-common.md#7-ipaddr) |
| Config{v}[].Port | Integer | Instance port | Y | See [Port](../00-common.md#1-port) type definition | Type is [Port](../00-common.md#1-port) |
| Config{v}[].Weight | Integer | Instance weight | Y | See [Weight](../00-common.md#8-weight) type definition | Type is [Weight](../00-common.md#8-weight); must be >= 0 |

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
