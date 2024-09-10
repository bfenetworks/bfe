# 请求Cookie相关条件原语

## req_cookie_key_in(key_list)

* 含义： 判断Cookie key是否为key_list之一
* 参数

| 参数     | 描述                   |
| -------- | ---------------------- |
| key_list | String<br>key列表，多个之间使用‘&#124;’连接 |

* 示例

```go
req_cookie_key_in("uid|cid|uss")
```

## req_cookie_value_in(key, value_list, case_insensitive)

* 含义： 判断cookie中key对应的值是否为value_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| key | String<br>cookie中的key |
| value_list | String<br>value列表，多个之间使用‘&#124;’连接 |
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例

```go
req_cookie_value_in("deviceid", "testid", true)
```

## req_cookie_value_prefix_in(key, prefix_list, case_insensitive)

* 含义： 判断cookie中key的值是否前缀匹配prefix_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| key | String<br>cookie中的key |
| prefix_list | String<br>prefix列表，多个之间使用‘&#124;’连接 |
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例

```go
req_cookie_value_prefix_in("deviceid", "x", true)
```

## req_cookie_value_suffix_in(key, suffix_list, case_insensitive)

* 含义： 判断cookie中key的值是否后缀匹配suffix_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| key | String<br>cookie中的key |
| suffix_list | String<br>suffix列表，多个之间使用‘&#124;’连接 |
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例

```go
req_cookie_value_suffix_in("deviceid", "1", true)
```

## req_cookie_value_hash_in(key, hash_value_list, case_insensitive)

* 含义： 对cookie中key的值哈希取模，判断是否匹配hash_value_list之一（模值0～9999）
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| key | String<br>cookie中的key |
| hash_value_list | String<br>hash value列表，多个之间使用‘&#124;’连接 |
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例

```go
req_cookie_value_hash_in("uid", "100", true)
```

## req_cookie_value_contain(key, value_list, case_insensitive)

* 含义： 判断cookie中key的值是否包含value_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| key | String<br>cookie中的key |
| value_list | String<br>value列表，多个之间使用‘&#124;’连接 |
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例

```go
req_cookie_value_contain("deviceid", "test", true)
```
