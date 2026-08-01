# mod_trust_clientip

## Introduction

mod_trust_clientip checks the client IP of incoming request/connnection against trusted ip dictionary. If matched, the request/connection is marked as trusted.

## Configuration

- [mod_trust_clientip.conf](../../configuration/mod_trust_clientip/mod_trust_clientip.conf.md)
- [trust_client_ip.data](../../configuration/mod_trust_clientip/trust_client_ip.data.md)

## Metrics

| Metric                       | Description                                        |
| ---------------------------- | -------------------------------------------------- |
| CONN_ADDR_INTERNAL           | Counter for connection from internal               |
| CONN_ADDR_INTERNAL_NOT_TRUST | Counter for connection from internal and not trust |
| CONN_TOTAL                   | Counter for all connnetion checked                 |
| CONN_TRUST_CLIENTIP          | Counter for connection from trust address         |
