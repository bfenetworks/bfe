# BFE Source Code Directory Structure

## Server
- `bfe_server`: implementation of core server 
- `bfe_basic`: defines basic data type
- `bfe_route`: implementation of routing
- `bfe_balance`: implementation of load balancing
- `bfe_config`: implementation of config
- `bfe_debug`: defines debug flags for important components 
- `bfe_util`: common utility functions

## Modules
- `bfe_module`: module framework
- `bfe_modules`: implementation of various modules

## Protocol
- `bfe_net`: common utility for net
- `bfe_http`: implementation of HTTP protocol
- `bfe_tls`:  implementation of TLS protocol
- `bfe_http2`: implementation of HTTP2 protocol
- `bfe_spdy`: implementation of SPDY protocol
- `bfe_stream`: implementation of TLS/TCP proxy
- `bfe_websocket`: implementation WebSocket protocl
- `bfe_proxy`: implementation of Proxy protocol
