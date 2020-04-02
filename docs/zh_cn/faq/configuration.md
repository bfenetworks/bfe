# 配置常见问题

## 如何启用TLS客户端认证
- 具体见[TLS客户端认证示例](https://github.com/baidu/bfe/blob/develop/docs/zh_cn/example/client_auth.md)

## 如何启用HTTP2协议
- 参考[tls_rule_conf.data](https://github.com/baidu/bfe/blob/develop/conf/tls_conf/tls_rule_conf.data)配置示例
    1. 针对某些VIP启用H2协议，配置NextProtos字段
    2. 针对所有VIP启用H2协议，配置DefaultNextProtos字段
- 对于方法1，需要4层负载均衡服务（可使用HAproxy，可参考客户端双向认证中[HAproxy配置示例](https://github.com/baidu/bfe/blob/develop/docs/zh_cn/example/client_auth.md)），通过PROXY协议将VIP透传给BFE