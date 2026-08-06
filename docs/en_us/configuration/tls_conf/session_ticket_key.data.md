# Configuration about TLS Session Ticket Key

## Introduction

session_ticket_key.data records the session ticket key.

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | See [Version](../00-common.md#5-version) type definition | Type must be [Version](../00-common.md#5-version) |
| SessionTicketKey | String | The session ticket key | Y | 96-character hexadecimal string representing 48 raw bytes; characters are limited to 0-9 and a-f<br>Also supports a raw 48-byte binary file as a fallback when JSON parsing fails | 96-character hexadecimal string (0-9, a-f), or a 48-byte raw key binary file |

## Example

```json
{
    "Version": "20190101000000",
    "SessionTicketKey": "08a0d852ef494143af613ef32d3c39314758885f7108e9ab021d55f422a454f7c9cd5a53978f48fa1063eadcdc06878f"
}
```
