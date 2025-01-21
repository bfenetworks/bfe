# mod_doh

## Introduction

Module doh supports DNS over HTTPS.

## Module configuration

### Description

conf/mod_doh/mod_doh.conf

| Config Item         | Description                                 |
| ------------------- | ------------------------------------------- |
| Basic.Cond          | String<br>Condition for DoH requests, see [Condition](../../condition/condition_grammar.md) |
| Dns.Address         | String<br>Address of DNS server |
| Dns.RetryMax        | Int<br>Maximum retries <br>Defaults to 0 (no retry) |
| Dns.Timeout         | Int<br>A cumulative timeout for dial, write and read (ms) |
| Log.OpenDebug       | Boolean<br>Whether enable debug log<br>Defaults to False |

### Example

```ini
[Basic]
Cond = "default_t()"

[Dns]
Address = "127.0.0.1:53"
Timeout = 1000

[Log]
OpenDebug = false
```
