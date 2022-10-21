# TLS相关条件原语

## ses_tls_sni_in(host_list)

* 语义: 判断TLS握手中的sni字段是否为host_list之一

* 参数

| 参数      | 描述                   |
| --------- | ---------------------- |
| host_list | String<br>域名列表, 多个域名之间使用&#124;分隔 |

* 示例

```go
ses_tls_sni_in("example.com|example.org")
```

## ses_tls_client_auth()

* 语义: 判断是否启用TLS双向认证

## ses_tls_client_ca_in(ca_list)

* 语义: 判断是否启用TLS双向认证且客户端证书签发根CA为ca_list之一

* 参数

| 参数      | 描述                   |
| --------- | ---------------------- |
| ca_list | String<br>CA标识列表, 多个CA标识之间使用&#124;分隔 |

* 示例

```go
ses_tls_client_ca_in("ca1|ca2")
```
