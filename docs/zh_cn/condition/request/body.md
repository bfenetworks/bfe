# 请求body相关条件原语

## req_body_json_in(json_path, value_list, case_insensitive)

* 含义： 在json格式的请求body中，查找json_path指定的字段，判断其值是否精确匹配value_list之一
* 参数  

| 参数     | 描述                   |
| -------- | ---------------------- |
| json_path | String<br>请求body中的json字段的路径 |
| value_list | String<br>value列表，多个之间使用‘&#124;’连接 |  
| case_insensitive | Boolean<br>是否忽略大小写 |

* 示例

```go
req_body_json_in("model", "deepseek-r1|qwen-plus", true)
```
