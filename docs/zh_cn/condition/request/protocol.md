# 请求协议相关条件原语

## req_proto_match(proto)

* 语义: 判断请求协议是否为proto
* 参数

| 参数   | 描述                   |
| ------ | ---------------------- |
| proto | String<br>协议名称，如 "https", "http", "spdy/3.1", "h2" 等，多个之间使用‘&#124;’连接 |

* 示例

```go
req_proto_match("https|h2")
```

## req_proto_secure()

* 语义: 判断请求是否基于TLS安全传输协议，包括HTTPS/SPDY/HTTP2
