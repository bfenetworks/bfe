# 请求标签相关条件原语

## req_tag_match(tagName, tagValue)

* 含义: 判断请求标签tagName的值是否为tagValue
注：请求在处理过程中可能会设置一些标签; 例: 请求在经过词典模块处理后，设置clientIP标签的值为blocklist

* 参数

| 参数      | 描述                   |
| --------- | ---------------------- |
| tagName   | String<br>标签名称     |
| tagValue  | String<br>标签取值     |

* 示例

```go
req_tag_match("clientIP", "blocklist")
```
