# Response code related primitives

## res_code_in(codes)

* Description: Judge response HTTP status code is in configured codes

* Parameters

| Parameter | Description |
| --------- | ---------- |
| codes | String<br>a list of codes which are concatenated using &#124; |

* Example

```go
res_code_in("200|500")
```
