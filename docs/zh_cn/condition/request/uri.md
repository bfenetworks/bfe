# 请求URI相关条件原语
## req_host_in(host_list)
* 含义： 判断http的host是否为host_list之一
    * 注：忽略大小写精确匹配
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| host_list | String<br>host列表，host之间使用‘&#124;’连接 |  

* 示例
```go
req_host_in("www.bfe-networks.com|bfe-networks.com")
```

## req_path_in(path_list, case_insensitive)
* 含义： 判断http的path是否为path_list之一

* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| path_list | String<br>path列表，多个path之间使用‘&#124;’连接 <br>每个path应以"/"开头 |
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例
```go
req_path_in("/api/search|/api/list", true)
```

## req_path_prefix_in(prefix_list, case_insensitive)
* 含义： 判断http的path是否前缀匹配prefix_list之一

* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| prefix_list | String<br>path prefix列表, 多个之间使用‘&#124;’连接 <br>每个path prefix应以"/"开头 |  
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例
```go
req_path_prefix_in("/api/report|/api/analytics", false)
```

## req_path_element_prefix_in(prefix_list, case_insensitive)
* 含义：判断http的path element是否前缀匹配prefix_list之一

* 参数

| 参数     | 描述                   |
| -------- | ---------------------- |
| prefix_list | String<br>path element prefix列表, 多个之间使用‘&#124;’连接 <br>每个path prefix应以"/"开头且以"/"结尾，非"/"结尾时会自动补充"/" |  
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例
```go
req_path_element_prefix_in("/api/report/|/api/analytics/", false)
```

## req_path_suffix_in(suffix_list, case_insensitive)
* 含义： 判断http的path是否后缀匹配suffix_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| suffix_list | String<br>path suffix列表，多个之间使用‘&#124;’连接 |  
| case_insensitive | Boolean<br>是否忽略大小写 |  

* 示例
```go
req_path_suffix_in(".php|.jsp", false)
```

## req_query_key_in(key_list)
* 含义： 判断请求query key是否为key_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| key_list | String<br>query key列表, 多个之间使用‘&#124;’连接 |  

* 示例
```go
req_query_key_in("word|wd")
```

## req_query_key_prefix_in(prefix_list)
* 含义： 判断query key是否为前缀匹配prefix_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| prefix_list | String<br>key prefix列表, 多个之间使用‘&#124;’连接 |  

* 示例
```go
req_query_key_prefix_in("rid")
```

## req_query_value_in(key, value_list, case_insensitive)
* 含义： 判断query中key的值是否精确匹配value_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| key | String<br>query中的key |
| value_list | String<br>value列表，多个之间使用‘&#124;’连接 |  
| case_insensitive | Boolean<br>是否忽略大小写 | 

* 示例
```go
req_query_value_in("uid", "x|y|z", true)
```

## req_query_value_prefix_in(key, prefix_list, case_insensitive)
* 含义： 判断query中key的值是否前缀匹配prefix_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| key | String<br>query中的key |
| prefix_list | String<br>prefix列表，多个之间使用‘&#124;’连接 |  
| case_insensitive | Boolean<br>是否忽略大小写 | 

* 示例
```go
req_query_value_prefix_in("uid", "100|200", true)
```

## req_query_value_suffix_in(key, suffix_list, case_insensitive)
* 含义： 判断query中key的值是否后缀匹配suffix_list之一
* 参数  
 
| 参数     | 描述                   |
| -------- | ---------------------- |
| key | String<br>query中的key |
| suffix_list | String<br>suffix列表, 多个之间使用‘&#124;’连接 |  
| case_insensitive | Boolean<br>是否忽略大小写 | 
 
* 示例
```go
req_query_value_suffix_in("uid", "1|2|3", true)
```

## req_query_value_hash_in(key, hash_value_list, case_insensitive)
* 含义： 对query中key的值哈希取模，判断是否匹配hash_value_list之一（模值0～9999）
* 参数  
 
| 参数     | 描述                   |
| -------- | ---------------------- |
| key | String<br>query中的key |
| hash_value_list | String<br>hash value列表, 多个之间使用‘&#124;’连接 |  
| case_insensitive | Boolean<br>是否忽略大小写 | 
 
* 示例
```go
req_query_value_hash_in("cid", "100", true)
```

## req_port_in(port_list)
* 含义： 判断请求端口是否为port_list之一
* 参数  
 
| 参数     | 描述                   |
| -------- | ---------------------- |
| port_list | String<br>port列表，多个port之间使用‘&#124;’连接 |  
 
* 示例
```go
req_port_in("80|8080")
```

## req_url_regmatch(reg_exp)
* 含义： 判断 url 是否匹配正则表达式reg_exp
    * 注： 推荐使用反引号，不需要额外进行转义
* 参数  
 
| 参数     | 描述                   |
| -------- | ---------------------- |
| reg_exp | String<br>表示正则表达式 |  
 
* 示例
```go
req_url_regmatch(`/s\?word=123`)
```
