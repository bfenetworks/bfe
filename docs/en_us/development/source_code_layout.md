# BFE Source Code Directory Structure

## Protocol

- `bfe_net`: common utility for net
- `bfe_http`: implementation of HTTP protocol
- `bfe_tls`:  implementation of TLS protocol
- `bfe_http2`: implementation of HTTP2 protocol
- `bfe_spdy`: implementation of SPDY protocol
- `bfe_stream`: implementation of TLS/TCP proxy
- `bfe_websocket`: implementation WebSocket protocol
- `bfe_proxy`: implementation of Proxy protocol

## Routing and Balancing

- `bfe_route`: implementation of routing
- `bfe_balance`: implementation of load balancing

## Modules

- `bfe_module`: module framework
- `bfe_modules`: implementation of various modules

## Server

- `bfe_server`: implementation of core server

## Utils

- `bfe_basic`: defines basic data type
- `bfe_config`: implementation of config
- `bfe_debug`: defines debug flags for important components
- `bfe_util`: common utility functions
