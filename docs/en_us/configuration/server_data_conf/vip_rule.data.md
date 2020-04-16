# Introduction

vip_rule.data records vip lists for each product. 

# Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of config file                                       |
| Vips        | Struct<br>Vip list for each product                                    |

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



