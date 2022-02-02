# Session IP related primitives

## ses_sip_range(start_ip, end_ip)

* Description: Judge if source IP of session is in [start_ip, end_ip]

* Parameter

| Parameter | Description |
| --------- | ---------- |
| start_ip | String<br>start ip address |
| end_ip | String<br>end ip address |

* Example

```go
ses_sip_range("10.0.0.1", "10.0.0.10")
```

## ses_vip_range(start_ip, end_ip)

* Description: Judge if VIP of session is in [start_ip, end_ip]

* Parameter

| Parameter | Description |
| --------- | ---------- |
| start_ip | String<br>start ip address |
| end_ip | String<br>end ip address |

* Example

```go
ses_vip_range("10.0.0.1", "10.0.0.10")
```
