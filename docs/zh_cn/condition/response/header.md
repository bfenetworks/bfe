# 响应头部相关条件原语

## res_header_key_in(key_list)

* 语义: 判断响应头部中key是否满足key_list之一

* 参数

| 参数      | 描述                   |
| --------- | ---------------------- |
| key_list  | String<br>Header Key列表, 多个Key之间使用&#124;分隔 |

* 示例

```go
res_header_key_in("X-Bfe-Debug")
```

## res_header_value_in(key, value_list, case_insensitive)

* 语义: 判断header中key值是否满足value_list之一

* 参数

| 参数             | 描述                   |
| ---------------- | ---------------------- |
| key              | String<br>Header Key   |
| value_list       | String<br>Header Value列表，多个Value之间使用&#124;分隔 |
| case_insensitive | Boolean<br>忽略大小写  |

* 示例

```go
res_header_value_in("X-Bfe-Debug", "1", true)
```
  