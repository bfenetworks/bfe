# VIP Rule Configuration

## Introduction

vip_rule.data records vip lists for each product.

## Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of config file                                       |
| Vips        | Object<br>Vip list for each product                                    |
| Vips{k}     | String<br>Product name                                                 |
| Vips{v}     | []String<br>VIP list for product                                       |
| Vips{v}[]   | String<br>VIP                                                          |

## Example

```json
{
    "Version": "20190101000000",
    "Vips": {
        "example_product": [
            "111.111.111.111"
        ] 
    }
}
```
