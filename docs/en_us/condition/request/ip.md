# IP Related Primitives

## req_cip_range(start_ip, end_ip)

* Description: Judge if client IP is in [start_ip, end_ip]

* Parameters

| Parameter | Description |
| --------- | ---------- |
| start_ip| String<br>start ip address |
| end_ip| String<br>end ip address |

* Example

```go
req_cip_range("10.0.0.1", "10.0.0.10")
```

## req_cip_trusted()

* Description: Judge if client IP is trust IP

## req_cip_hash_in(value_list)

* Description:
  - Judge if client IP after hash matches configured patterns (value after hash is 0～9999)

* Parameters

| Parameter | Description |
| --------- | ---------- |
| value_list | String<br>a list of hash values which are concatenated using &#124; |

* Example

```go
req_cip_hash_in("100")
req_cip_hash_in("100-200")
req_cip_hash_in("100-200|1000-1000")
```

## req_vip_in(vip_list)

* Description: Judge if VIP is in configured VIP list

* Parameters

| Parameter | Description |
| --------- | ---------- |
| vip_list | String<br>a list of vips which are concatenated using &#124; |

* Example

```go
req_vip_in("10.0.0.1|10.0.0.2")
```

## req_vip_range(start_ip, end_ip)

* Description: Judge if VIP is in [start_ip, end_ip]

* Parameters

| Parameter | Description |
| --------- | ---------- |
| start_ip| String<br>start ip address |
| end_ip| String<br>end ip address |

* Example

```go
req_vip_range("10.0.0.1", "10.0.0.10")
```
