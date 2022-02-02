# Response header related primitives

## res_header_key_in(key_list)

* Description: Judge if key in Header of response matches configured key_list

* Parameters

| Parameter | Description |
| --------- | ---------- |
| key_list | String<br>a list of header keys which are concatenated using &#124; |

* Example

```go
res_header_key_in("X-Bfe-Debug")
```

## res_header_value_in(key, value_list, case_insensitive)

* Description: Judge if value of key in response header matches configured patterns

* Parameters

| Parameter | Description |
| --------- | ---------- |
| key       | String<br>header name |
| value_list | String<br>a list of header values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```go
res_header_value_in("X-Bfe-Debug", "1", true)
```
