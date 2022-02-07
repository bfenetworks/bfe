# VIP Rule Configuration

## Introduction

vip_rule.data records vip lists for each product.

## Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of config file                                       |
| Vips        | Struct<br>Vip list for each product                                    |
| Vips{k}     | String<br>Product name                                                 |
| Vips{v}     | Struct<br>Vip list for product                                         |

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
