# Cookie

## Common Condition Primitive Parameter

- key: String, the key in cookie
- patterns: String, representing multiple patterns, format is as "pattern1|pattern2"
- case_insensitive: Bool, if ignore the case-sensitive of value

## Condition Primitive About Cookie

- **req_cookie_key_in(patterns)**
  - Judge if cookie key matches configured patterns
  
    ```
    # if cookie key is UID
    req_cookie_key_in(“UID”)
    ```
- **req_cookie_value_in(key, patterns, case_insensitive)**
  - Judge if value of key in cookie matches configured patterns
  
    ```
    # if the value of UID(case-insensitive) in cookie is XXX
    req_cookie_value_in(“UID”, "XXX", true)
    ```
- **req_cookie_value_prefix_in(key, patterns, case_insensitive)**
  
  - Judge if value prefix of key in cookie matches configured patterns
  
    ```
    # if the value prefix of UID(case-insensitive) in cookie is XXX
    req_cookie_value_prefix_in(“UID”, "XXX", true)
    ```
- **req_cookie_value_suffix_in(key, patterns, case_insensitive)**
  
  - Judge if value suffix of key in cookie matches configured patterns
  
    ```
    # if the value suffix of UID(case-insensitive) in cookie is XXX
    req_cookie_value_suffix_in(“UID”, "XXX", true)
    ```
- **req_cookie_value_hash_in(key, patterns, case_insensitive)**
  - Judge if value of key after hash in cookie matches configured patterns (value after hash is 0～9999)
  
    ```
    # if the value after hash of UID(case-insensitive) in cookie is 100
    req_cookie_value_hash_in(“UID”, “100”, true)
    ```
- **req_cookie_value_contain(key, patterns, case_insensitive)**
  - Judge if value of key in cookie contains configured patterns

    ```
    # if the value of UID(case-insensitive) contains cookie is XXX
    req_cookie_value_contain(“UID”, "XXX", true)
    ```
    

