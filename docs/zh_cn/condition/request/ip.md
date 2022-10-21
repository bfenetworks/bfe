# 请求IP相关条件原语

## req_cip_range(start_ip, end_ip)

* 含义：判断请求的clientip是否在 [start_ip, end_ip] 区间内

* 参数

| 参数     | 描述                   |
| -------- | ---------------------- |
| start_ip | String<br>起始IP地址   |
| end_ip   | String<br>结束IP地址   |

* 示例

```go
req_cip_range("10.0.0.1", "10.0.0.10")
```

## req_cip_trusted()

* 含义：判断clientip是否为信任IP

## req_cip_hash_in(value_list)

* 含义：对cip哈希取模，判断值是否匹配value_list

* 参数

| 参数      | 描述                   |
| --------- | ---------------------- |
| value_list | String<br>哈希值列表, 多个元素之间使用&#124;分隔; <br>列表中每个元素，可以是单个数值，或取值范围;<br>哈希值范围0~9999 |

* 示例

```go
req_cip_hash_in("100")
req_cip_hash_in("100-200")
req_cip_hash_in("100-200|1000-1100")
```

## req_vip_in(vip_list)

* 含义: 判断访问VIP是否在指定vip_list中

* 参数

| 参数      | 描述                   |
| --------- | ---------------------- |
| vip_list   | String<br>VIP列表, 多个VIP之间使用&#124;分隔 |

* 示例

```go
req_vip_in("10.0.0.1|10.0.0.2")
```

## req_vip_range(start_ip, end_ip)

* 含义: 判断访问VIP是否在指定 [start_ip, end_ip] 区间内

* 参数

| 参数     | 描述                   |
| -------- | ---------------------- |
| start_ip | String<br>起始IP地址   |
| end_ip   | String<br>结束IP地址   |

* 示例

```go
req_vip_range("10.0.0.1", "10.0.0.10")
```
