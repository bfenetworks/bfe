# mod_trust_clientip Trusted IP Dictionary Configuration

## Configuration Introduction

`trust_client_ip.data` is the trusted IP dictionary configuration file for the `mod_trust_clientip` module, used to configure all trusted IP segments.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Trusted IP config | Y | Key is address label | - |
| Config{k} | String | Address label | Y | - | - |
| Config{v} | Array | A list of IP segments | Y | - | - |
| Config{v}[] | Object | An IP segment | Y | - | - |
| Config{v}[].Begin | String | Start IP address | Y | - | Type is [IPAddr](../00-common.md#7-ipaddr); must be less than or equal to the end address |
| Config{v}[].End | String | End IP address | Y | - | Type is [IPAddr](../00-common.md#7-ipaddr); must be greater than or equal to the start address |

## Configuration Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "inner-idc": [
            {
                "Begin": "10.0.0.0",
                "End": "10.255.255.255"
            }
        ]
    }
}
```
