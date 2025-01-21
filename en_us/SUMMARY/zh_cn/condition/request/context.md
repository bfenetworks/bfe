## req_context_value_in(key, value_list, case_insensitive)

* 含义： 判断请求context中key的值是否精确匹配value_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| key | String<br>context中的key |
| value_list | String<br>value列表，多个之间使用‘&#124;’连接 |  
| case_insensitive | Boolean<br>是否忽略大小写 |

* 示例

```go
req_context_value_in("cmd", "add|del|list", true)
```
