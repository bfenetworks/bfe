# Request cookie related primitive

## req_cookie_key_in(key_list)
* Description: Judge if cookie key matches configured patterns

* Parameters

| Parameter | Descrption |
| --------- | ---------- |
| key_list | String<br>a list of cookie keys which are concatenated using &#124; |


* Example

```
req_cookie_key_in("UID")
```

## req_cookie_value_in(key, value_list, case_insensitive)
* Description: Judge if value of cookie key matches configured patterns

* Parameters

| Parameter | Descrption |
| --------- | ---------- |
| key | String<br>cookie key |
| value_list | String<br>a list of hash values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```
req_cookie_value_in("UID", "XXX", true)
```

## req_cookie_value_prefix_in(key, value_prefix_list, case_insensitive)
* Description: Judge if value prefix of cookie key matches configured patterns

* Parameters

| Parameter | Descrption |
| --------- | ---------- |
| key | String<br>cookie key |
| value_prefix_list | String<br>a list of values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```
req_cookie_value_prefix_in("UID", "XXX", true)
```

## req_cookie_value_suffix_in(key, value_suffix_list, case_insensitive)
* Description: Judge if value suffix of cookie key matches configured patterns

* Parameters

| Parameter | Descrption |
| --------- | ---------- |
| key | String<br>cookie key |
| value_suffix_list | String<br>a list of values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```
req_cookie_value_suffix_in("UID", "XXX", true)
```

## req_cookie_value_hash_in(key, value_list, case_insensitive)
* Description: Judge if hash value of specified cookie matches configured patterns(value range: 0～9999)

* Parameters

| Parameter | Descrption |
| --------- | ---------- |
| key | String<br>cookie key |
| value_list | String<br>a list of hash values which are concatenated using &#124; |
| case_insensitive | Boolean<br>case insensitive |

* Example

```
req_cookie_value_hash_in("UID", "100", true)
```

## req_cookie_value_contain(key, value, case_insensitive)
* Description: Judge if value of cookie key contains configured patterns

* Parameters

| Parameter | Descrption |
| --------- | ---------- |
| key | String<br>cookie key |
| value | String<br>a string |
| case_insensitive | Boolean<br>case insensitive |


* Example

```
req_cookie_value_contain("UID", "XXX", true)
```
