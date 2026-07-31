# Protocol Related Primitives

## req_proto_match(proto)

* Description: Judge if request protocol matches configured protocols

* Parameters

| Parameter | Description |
| --------- | ---------- |
| proto | String<br>a list of protocol names which are concatenated using &#124;<br>e.g. "https", "http", "spdy/3.1", "h2" |

* Example

```go
req_proto_match("https|h2")
```

## req_proto_secure()

* Description: Judge if request is over TLS protocol(ie. HTTPS/SPDY/HTTP2)
