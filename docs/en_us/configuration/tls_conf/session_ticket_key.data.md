# Configuration about TLS Session Ticket Key

## Introduction

session_ticket_key.data records the session ticket key.

## Configuration

| Config Item      | Description                                                     |
| ---------------- | --------------------------------------------------------------- |
| Version          | String<br>Version of config file                                          |
| SessionTicketKey | String<br>The session ticket key. length is 48 and contains only [a-z0-9] |

## Example

```json
{
    "Version": "20190101000000",
    "SessionTicketKey": "08a0d852ef494143af613ef32d3c39314758885f7108e9ab021d55f422a454f7c9cd5a53978f48fa1063eadcdc06878f"
}
```
