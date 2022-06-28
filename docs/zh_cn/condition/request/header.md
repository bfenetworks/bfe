# 请求头部相关条件原语

## req_header_key_in(key_list)

* 含义： 判断请求头部中key是否为key_list之一

* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| key_list | String<br>key列表, 多个之间使用‘&#124;’连接<br>Header名称使用HTTP协议规范形式|  

* 示例

```go
// 正确：
req_header_key_in("Header-Test")
  
// 错误：
req_header_key_in("Header-test")
req_header_key_in("header-test")
req_header_key_in("header-Test")
```

## req_header_value_in(header_name, value_list, case_insensitive)

* 含义： 判断http消息头部字段是否为value_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| header_name | String<br>请求header中的key |
| value_list | String<br>value列表，多个之间使用‘&#124;’连接 |  
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例

```go
req_header_value_in("Referer", "https://example.org/login", true)
```

## req_header_value_prefix_in(header_name, prefix_list, case_insensitive)

* 含义： 判断http消息头部字段值是否前缀匹配prefix_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| header_name | String<br>请求header中的key |
| prefix_list | String<br>prefix列表，多个之间使用‘&#124;’连接 |  
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例

```go
req_header_value_prefix_in("Referer", "https://example.org", true)
```

## req_header_value_suffix_in(header_name, suffix_list, case_insensitive)

* 含义： 判断http消息头部字段值是否后缀匹配suffix_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| header_name | String<br>请求header中的key |
| suffix_list | String<br>suffix列表，多个之间使用‘&#124;’连接 |  
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例

```go
req_header_value_suffix_in("User-Agent", "2.0.4", true)
```

## req_header_value_hash_in(header_name, hash_value_list, case_insensitive)

* 含义： 对http消息头部字段值哈希取模，判断是否匹配hash_value_list之一（模值0～9999）
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| header_name | String<br>请求header中的key |
| hash_value_list | String<br>hash value列表，多个之间使用‘&#124;’连接 |  
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例

```go
req_header_value_hash_in("X-Device-Id", "100-200|400", true)
```

## req_header_value_contain(header_name, value_list, case_insensitive)

* 含义： 判断http消息头部字段值是否包含value_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| header_name | String<br>请求header中的key |
| value_list | String<br>value列表，多个之间使用‘&#124;’连接 |
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例

```go
req_header_value_contain("User-Agent", "Firefox|Chrome", true)
```
