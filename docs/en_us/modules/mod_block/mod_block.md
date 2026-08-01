# mod_block

## Introduction

mod_block blocks incoming connections/requests based on defined rules.

## Module Configuration

For module configuration, see [mod_block.conf](../../configuration/mod_block/mod_block.conf.md).

## Rule Configuration

For block rule configuration, see [block_rules.data](../../configuration/mod_block/block_rules.data.md).

For global IP blocklist configuration, see [ip_blocklist.data](../../configuration/mod_block/ip_blocklist.data.md).

## Metrics

| Metric        | Description                                                  |
| ------------- | ------------------------------------------------------------ |
| CONN_ACCEPT   | Counter for connection accepted                              |
| CONN_REFUSE   | Counter for connection refused                               |
| CONN_TOTAL    | Counter for all connnetion checked                           |
| REQ_ACCEPT    | Counter for request accepted                                 |
| REQ_REFUSE    | Counter for request refused                                  |
| REQ_TOTAL     | Counter for all request in                                   |
| WRONG_COMMAND | Counter for request with condition satisfied, but wrong command |
