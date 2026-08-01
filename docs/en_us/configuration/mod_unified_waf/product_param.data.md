# mod_unified_waf WAF Product Configuration

## Introduction

`product_param.data` is used to configure behavior parameters for each product line during WAF detection.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | - | Type is [Version](../00-common.md#5-version) |
| Config | Object | Product configuration | Y | - | Must be a non-empty object |
| Config{k} | String | Product name | Y | Key of `Config` | Non-empty string |
| Config{v} | Object | Configuration of the product | Y | - | Must be a non-empty object |
| Config{v}.SendBody | Boolean | Whether to send body during WAF detection | Y | - | - |
| Config{v}.SendBodySize | Integer | Max body size to send during WAF detection | Y | Unit: byte | Non-negative integer |

## Configuration Example

```json
{
    "Version": "2023-01-19 12:00:10",
    "Config": {
        "example_product": {
            "SendBody": true,
            "SendBodySize": 1024
        }
    }
}
```
