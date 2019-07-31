# Introduction

host_rule.data records the domain names for each product. 

# Configuration

| Config Item    | Type   | Description                                                  |
| -------------- | ------ | ------------------------------------------------------------ |
| Version        | String | Verson of config file                                        |
| DefaultProduct | String | Default product name.                                        |
| HostTags       | Struct | HostTag list for each product                                |
| Hosts          | Struct | Host list for each HostTag                                   |

# Example

```
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



