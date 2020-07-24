# Stream

## Introduction

The endport `/monitor/stream_state` exposes metrics about TLS-TCP reverse proxy.

## Metrics

| Metric              | Description                          |
| ------------------- | ------------------------------------ |
| STREAM_BYTES_RECV   | Counter for receiving stream bytes   |
| STREAM_BYTES_SENT   | Counter for sending stream bytes     |
| STREAM_ERR_BALANCE  | Counter for balance error            |
| STREAM_ERR_CONNECT  | Counter for connecting backend error |
| STREAM_ERR_PROXY    | Counter for finding backend error    |
| STREAM_ERR_TRANSFER | Counter for transfer error           |
| STREAM_PANIC_CONN   | Counter for connection panic         |
