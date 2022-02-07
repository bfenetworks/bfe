# Request Method Related Primitives

## req_method_in(method_list)

* Description: Judge if request method matches configured patterns

* Parameters

| Parameter | Description |
| --------- | ----------- |
| method_list | String<br>a list of methods which are concatenated by &#124;. <br>valid method:GET/POST/PUT/DELETE |

* Example

```go
req_method_in("GET|POST")
```
