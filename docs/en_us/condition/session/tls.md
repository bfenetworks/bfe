# TLS related primtives

## ses_tls_sni_in(host_list)

* Description: Check whether tls sni matches host_list

* Parameters

| Parameter | Description |
| --------- | ----------- |
| host_list | String<br>a list of hosts which are concatenated using &#124; |

* Example

```go
ses_tls_sni_in("example.com|example.org")
```

## ses_tls_client_auth()

* Description: Check whether tls mutual authentication is enabled

## ses_tls_client_ca_in(ca_list)

* Description: Check whether tls mutual authentication is enabled and client ca matches ca_list

* Parameters

| Parameter | Description |
| --------- | ----------- |
| ca_list | String<br>a list of ca names which are concatenated using &#124; |

* Example

```go
ses_tls_client_ca_in("ca1|ca2")
```
