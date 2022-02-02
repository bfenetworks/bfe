# Request Header Related Primitives

## req_header_key_in(key_list)

* Description: Judge if header key in matches configured patterns

* Parameters

| Parameter | Description |
| --------- | ---------- |
| key_list | String<br>a list of header keys which are concatenated using &#124;<br>The header key should be in canonical form |

* Example

```go
// right：
req_header_key_in("Header-Test")

// wrong：
req_header_key_in("Header-test")
req_header_key_in("header-test")
req_header_key_in("header-Test")
```
  
## req_header_value_in(header_name, value_list, case_insensitive)

* Description:
  - Judge if value of key in header matches configured patterns

* Parameters

| Parameter | Description |
| --------- | ---------- |
| header_name | String<br>header name |
| value_list | String<br>a list of header values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```go
req_header_value_in("Referer", "https://example.org/login", true)
```

## req_header_value_prefix_in(header_name, value_prefix_list, case_insensitive)

* Description: Judge if value prefix of key in header matches configured patterns

* Parameters

| Parameter | Description |
| --------- | ---------- |
| header_name | String<br>header name |
| value_prefix_list | String<br>a list of values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```go
req_header_prefix_value_in("Referer", "https://example.org", true)
```

## req_header_value_suffix_in(header_name, value_suffix_list, case_insensitive)

* Description: Judge if value suffix of key in header matches configured patterns

* Parameters

| Parameter | Description |
| --------- | ---------- |
| header_name | String<br>header name |
| value_suffix_list | String<br>a list of values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```go
req_header_suffix_value_in("User-Agent", "2.0.4", true)
```

## req_header_value_hash_in(header_name, value_list, case_insensitive)

* Description: Judge if hash value of specified header matches configured patterns (value range: 0～9999)

* Parameters

| Parameter | Description |
| --------- | ---------- |
| header_name | String<br>header name |
| value_list | String<br>a list of hash values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```go
req_header_value_hash_in("X-Device-Id", "100-200|400", true)
```

## req_header_value_contain(header_name, value_list, case_insensitive)

* Description: Judge if value of key in header contains configured patterns

* Parameters

| Parameter | Description |
| --------- | ---------- |
| header_name | String<br>header name |
| value_list | String<br>a list of hash values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```go
req_header_value_contain("User-Agent", "Firefox|Chrome", true)
```
