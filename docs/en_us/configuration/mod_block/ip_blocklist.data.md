# mod_block Global IP Blocklist Configuration

## Introduction

`ip_blocklist.data` is the global IP blocklist configuration file of the `mod_block` module, used to configure globally blocked IP addresses or IP ranges.

## Configuration Description

Each line configures one or more IP addresses; each line can contain a single IP, or a start IP and end IP of an IP range (separated by space).

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| IP Address | String | A single IP address | Y | Valid IPv4 or IPv6 address | Type is [IPAddr](../00-common.md#7-ipaddr) |
| Start IP | String | Start address of an IP range | Y | Must be configured in pair with End IP | Type is [IPAddr](../00-common.md#7-ipaddr); must be less than or equal to End IP |
| End IP | String | End address of an IP range | Y | Must be configured in pair with Start IP | Type is [IPAddr](../00-common.md#7-ipaddr); must be greater than or equal to Start IP |

## Configuration Example

```
192.168.1.253 192.168.1.254
192.168.1.250
```
