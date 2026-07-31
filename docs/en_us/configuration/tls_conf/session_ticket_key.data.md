# Configuration about TLS Session Ticket Key

## Introduction

session_ticket_key.data records the session ticket key.

## Configuration

| Config Item      | Description                                                     |
| ---------------- | --------------------------------------------------------------- |
| Version          | String<br>Version of config file                                          |
| SessionTicketKey | String<br>The session ticket key, a 96-character hexadecimal string representing 48 raw bytes; characters are limited to 0-9 and a-f<br>Also supports a raw 48-byte binary file as a fallback when JSON parsing fails |

## Example

```json
{
    "Version": "20190101000000",
    "SessionTicketKey": "08a0d852ef494143af613ef32d3c39314758885f7108e9ab021d55f422a454f7c9cd5a53978f48fa1063eadcdc06878f"
}
```
