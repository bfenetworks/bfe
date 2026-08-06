# VIP Rule Configuration

## Introduction

vip_rule.data records vip lists for each product.

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | See [Version](../00-common.md#5-version) type definition | Type is [Version](../00-common.md#5-version) |
| Vips | Object | VIP list for each product | Y | Key is product name, value is the VIP list of the product | Non-empty |
| Vips{k} | String | Product name | Y | Key of Vips | Non-empty |
| Vips{v} | []String | VIP list | Y | All VIPs under this product | Non-empty |
| Vips{v}[] | String | VIP | Y | See [IPAddr](../00-common.md#7-ipaddr) type definition | Type is [IPAddr](../00-common.md#7-ipaddr) |

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
