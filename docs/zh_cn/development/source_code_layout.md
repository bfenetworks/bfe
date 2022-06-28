# BFE源代码结构

## 接入协议

- `bfe_net`: BFE网络相关基础库代码
- `bfe_http`: BFE HTTP协议基础代码
- `bfe_tls`: BFE TLS协议基础代码
- `bfe_http2`: BFE HTTP2协议基础代码
- `bfe_spdy`: BFE SPDY协议基础代码
- `bfe_stream`:	BFE TLS代理基础代码
- `bfe_websocket`: BFE WebSocket代理基础代码
- `bfe_proxy`: BFE Proxy协议基础代码

## 分流转发

- `bfe_route`: BFE分流转发相关代码
- `bfe_balance`: BFE负载均衡相关代码

## 扩展模块

- `bfe_module`: BFE模块框架相关代码
- `bfe_modules`: BFE扩展模块相关代码

## 服务框架

- `bfe_server`: BFE服务端主体部分

## 基础工具

- `bfe_basic`: BFE基础数据类型定义
- `bfe_config`: BFE配置加载相关代码
- `bfe_debug`: BFE模块调试开关相关代码
- `bfe_util`: BFE基础库相关代码
