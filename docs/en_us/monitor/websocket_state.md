# WebSocket

## Introduction

The endpoint `/monitor/websocket_state` exposes metrics about WebSocket.

## Metrics

| Metric                        | Description                           |
| ----------------------------- | ------------------------------------- |
| WEB_SOCKET_BYTES_RECV         | Counter for receiving websocket bytes |
| WEB_SOCKET_BYTES_SENT         | Counter for sending websocket bytes   |
| WEB_SOCKET_ERR_BACKEND_REJECT | Counter for rejecting backend         |
| WEB_SOCKET_ERR_BALANCE        | Counter for balance error             |
| WEB_SOCKET_ERR_CONNECT        | Counter for connecting error          |
| WEB_SOCKET_ERR_HANDSHAKE      | Counter for handshake error           |
| WEB_SOCKET_ERR_PROXY          | Counter for finding backend           |
| WEB_SOCKET_ERR_TRANSFER       | Counter for transfer error            |
| WEB_SOCKET_PANIC_CONN         | Counter for connection panic          |
