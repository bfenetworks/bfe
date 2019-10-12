# Response Header

- **res_header_key_in(patterns)**
  - Judge if key in Header of response matches configured patterns
  
  - Patterns represent multiple patterns. The type of it is string, format is as "pattern1|pattern2"
  
    ```
    # if the key in header of response is Header-Test
    res_header_key_in(“Header-Test”)
    ```
- **res_header_value_in(key, patterns, case_insensitive)**
  - Judge if value of key in response header matches configured patterns
  
  - The type of key is string
  
  - Patterns represent multiple patterns. The type of it is string, format is as "pattern1|pattern2"
  
    ```
    # if the value of Header-Test in header is XXX
    res_header_value_in("Header-Test", "XXX", true)
    ```
