# Header

## Common Condition Primitive Parameter

- header_name: String, the key in header
- patterns: String, representing multiple patterns, format is as "pattern1|pattern2"
- case_insensitive: Bool, case sensitive

## Condition Primitive About Header

- **req_header_key_in(patterns)**
  - Judge if header key in matches configured patterns
  - **Note: each word in header key need to be capitalized**
   ```
   # right：
   req_header_key_in("Header-Test")

   # wrong：
   req_header_key_in("Header-test")
   req_header_key_in("header-test")
   req_header_key_in("header-Test")
   ```
  
- **req_header_value_in(header_name, patterns, case_insensitive)**
  - Judge if value of key in header matches configured patterns
  ```
  # if the value of Host in header is XXX.com
  req_header_value_in("Host", "XXX.com", true)
  ```

- **req_header_value_prefix_in(header_name, patterns, case_insensitive)**
  - Judge if value prefix of key in header matches configured patterns
  ```
  # if the value prefix of Host in header is XXX
  req_header_prefix_value_in("Host", "XXX", true)
  ```

- **req_header_value_suffix_in(header_name, patterns, case_insensitive)**
  - Judge if value suffix of key in header matches configured patterns
  ```
  # if the value suffix of Host in header is XXX
  req_header_suffix_value_in("Host", "XXX", true)
  ```

- **req_header_value_hash_in(header_name, patterns, case_insensitive)**
  - Judge if hash value of specified header matches configured patterns (value range: 0～9999)
  ```
  # if hash value of header Host is 100~200 or 400
  req_header_value_hash_in("Host", "100-200|400", true)
  ```

- **req_header_value_contain(header_name, patterns, case_insensitive)**
  - Judge if value of key in header contains configured patterns
  ```
  # if the value of Host in header contains XXX.com
  req_header_value_contain("Host", "XXX.com", true)
  ```
