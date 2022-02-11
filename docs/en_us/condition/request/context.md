## req_context_value_in(key, value_list, case_insensitive)

* Description: Judge if value of context key matches configured patterns

* Parameters

| Parameter | Description |
| --------- | ---------- |
| key | String<br> context key |
| value_list | String<br>a list of query values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```go
req_context_value_in("cmd", "add|del|list", true)
```
