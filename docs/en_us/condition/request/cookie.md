# Cookie

## Common Condition Primitive Parameter

- key: String, the key in cookie
- patterns: String, representing multiple patterns, format is as "pattern1|pattern2"
- case_insensitive: Bool, case insensitive

## Condition Primitive About Cookie

- **req_cookie_key_in(patterns)**
  - Judge if cookie key matches configured patterns
  ```
  # if cookie key is UID
  req_cookie_key_in("UID")
  ```

- **req_cookie_value_in(key, patterns, case_insensitive)**
  - Judge if value of cookie key matches configured patterns
  ```
  # if the value(case-insensitive) of cookie UID is XXX
  req_cookie_value_in("UID", "XXX", true)
  ```

- **req_cookie_value_prefix_in(key, patterns, case_insensitive)**
  - Judge if value prefix of cookie key matches configured patterns
  ```
  # if the value prefix(case-insensitive) of cookie UID is XXX
  req_cookie_value_prefix_in("UID", "XXX", true)
  ```

- **req_cookie_value_suffix_in(key, patterns, case_insensitive)**
  - Judge if value suffix of cookie key matches configured patterns
  ```
  # if the value suffix(case-insensitive) of cookie UID is XXX
  req_cookie_value_suffix_in("UID", "XXX", true)
  ```

- **req_cookie_value_hash_in(key, patterns, case_insensitive)**
  - Judge if hash value of specified cookie matches configured patterns(value range: 0～9999)
  ```
  # if hash value of cookie UID after hash is 100
  req_cookie_value_hash_in("UID", "100", true)
  ```

- **req_cookie_value_contain(key, patterns, case_insensitive)**
  - Judge if value of cookie key contains configured patterns
  ```
  # if the value(case-insensitive) of UID contains cookie is XXX
  req_cookie_value_contain("UID", "XXX", true)
  ```
