# Request Cookie Related Primitives

## req_cookie_key_in(key_list)

* Description: Judge if cookie key matches configured patterns

* Parameters

| Parameter | Description |
| --------- | ---------- |
| key_list | String<br>a list of cookie keys which are concatenated using &#124; |

* Example

```go
req_cookie_key_in("uid|cid|uss")
```

## req_cookie_value_in(key, value_list, case_insensitive)

* Description: Judge if value of cookie key matches configured patterns

* Parameters

| Parameter | Description |
| --------- | ---------- |
| key | String<br>cookie key |
| value_list | String<br>a list of hash values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```go
req_cookie_value_in("deviceid", "testid", true)
```

## req_cookie_value_prefix_in(key, value_prefix_list, case_insensitive)

* Description: Judge if value prefix of cookie key matches configured patterns

* Parameters

| Parameter | Description |
| --------- | ---------- |
| key | String<br>cookie key |
| value_prefix_list | String<br>a list of values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```go
req_cookie_value_prefix_in("deviceid", "x", true)
```

## req_cookie_value_suffix_in(key, value_suffix_list, case_insensitive)

* Description: Judge if value suffix of cookie key matches configured patterns

* Parameters

| Parameter | Description |
| --------- | ---------- |
| key | String<br>cookie key |
| value_suffix_list | String<br>a list of values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```go
req_cookie_value_suffix_in("deviceid", "1", true)
```

## req_cookie_value_hash_in(key, value_list, case_insensitive)

* Description: Judge if hash value of specified cookie matches configured patterns(value range: 0～9999)

* Parameters

| Parameter | Description |
| --------- | ---------- |
| key | String<br>cookie key |
| value_list | String<br>a list of hash values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```go
req_cookie_value_hash_in("uid", "100", true)
```

## req_cookie_value_contain(key, value, case_insensitive)

* Description: Judge if value of cookie key contains configured patterns

* Parameters

| Parameter | Description |
| --------- | ---------- |
| key | String<br>cookie key |
| value | String<br>a string |
| case_insensitive | Boolean<br>case insensitive |

* Example

```go
req_cookie_value_contain("deviceid", "test", true)
```
