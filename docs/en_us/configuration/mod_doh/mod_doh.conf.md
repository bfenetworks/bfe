# mod_doh Basic Configuration

## Introduction

`mod_doh.conf` is the basic configuration file of `mod_doh`, used to specify the matching condition for DoH requests, the DNS server address, and log options.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.Cond | String | Condition for DoH requests | Y | Syntax see [Condition](../../condition/condition_grammar.md) | Must be a valid Condition expression |
| Dns.Address | String | Address of DNS server | Y | Example: `127.0.0.1:53` | Must be a valid UDP address |
| Dns.RetryMax | Integer | Maximum retries | N | Defaults to `0` (no retry) | Value must be `>= 0` |
| Dns.Timeout | Integer | Cumulative timeout for DNS query, in milliseconds | Y | - | Value must be `> 0` |
| Log.OpenDebug | Boolean | Whether to enable debug log | N | Defaults to `False` | - |

## Example

```ini
[Basic]
Cond = "default_t()"

[Dns]
Address = "127.0.0.1:53"
Timeout = 1000

[Log]
OpenDebug = false
```
