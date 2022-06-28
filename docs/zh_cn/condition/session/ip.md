# 会话IP相关条件原语

## ses_sip_range(start_ip, end_ip)

* 语义: 判断会话的源ip是否在 [start_ip, end_ip] 区间内

* 参数

| 参数     | 描述                   |
| -------- | ---------------------- |
| start_ip | String<br>起始IP地址   |
| end_ip   | String<br>结束IP地址   |

* 示例

```go
ses_sip_range("10.0.0.1", "10.0.0.10")
```

## ses_vip_range(start_ip, end_ip)

* 语义: 判断访问VIP是否在 [start_ip, end_ip] 区间内

* 参数

| 参数     | 描述                   |
| -------- | ---------------------- |
| start_ip | String<br>起始IP地址   |
| end_ip   | String<br>结束IP地址   |

* 示例

```go
ses_vip_range("10.0.0.1", "10.0.0.10")
```
