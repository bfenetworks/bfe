# Introduction

vip_rule.data records vip lists for each product. 

# Configuration

| Config Item | Type   | Description                                                  |
| ----------- | ------ | ------------------------------------------------------------ |
| Version     | String | Version of config file                                       |
| Vips        | Struct | Vip list for each product                                    |

# Example

```
{
    "Version": "20190101000000",
    "Vips": {
        "example_product": [
            "111.111.111.111"
        ] 
    }
}
```



