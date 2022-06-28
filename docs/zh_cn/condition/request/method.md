# 请求方法相关条件原语

## req_method_in(method_list)

* 含义: 请求方法是否匹配method_list之一

* 参数

| 参数        | 描述                                               |
| ----------- | -------------------------------------------------- |
| method_list | String<br>请求方法列表，多个方法之间使用&#124;分隔<br>方法取值GET/POST/PUT/DELETE |

* 示例

```go
req_method_in("GET|POST")
```
