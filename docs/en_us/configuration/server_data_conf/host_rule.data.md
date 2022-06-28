# Host Rule Configuration

## Introduction

host_rule.data records the domain names for each product.

## Configuration

| Config Item    | Description                                                  |
| -------------- | ------------------------------------------------------------ |
| Version        | String<br>Verson of config file                                        |
| DefaultProduct | String<br>Default product name.                                        |
| Hosts          | Struct<br>Host list for each HostTag                                   |
| Hosts{k}       | Struct<br>HostTag                                                      |
| Hosts{v}       | String<br>Host list for HostTag                                        |
| HostTags       | Struct<br>HostTag list for each product                                |
| HostTags{k}    | Struct<br>Product name                                                 |
| HostTags{v}    | Struct<br>HostTag list for product                                     |

## Example

```json
{
    "Version": "20190101000000",
    "DefaultProduct": null,
    "Hosts": {
        "exampleTag":[
            "example.org"
        ]
    },
    "HostTags": {
        "example_product":[
            "exampleTag"
        ]
    }
}
```
