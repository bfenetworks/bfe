# URI

In general, URI format is as follows：

- http://host/path/?query

## Common Condition Primitive Parameter

- patterns: String, representing multiple patterns, format is as "pattern1|pattern2"
- case_insensitive: Bool, if ignore the case-sensitive of value

## Host

- **req_host_in(patterns)**
  - Judge if host matches configured patterns
  
  - Case insensitive
  
    ```
    // match www.bfe-networks.com or bfe-networks.com, case insensitive
    req_host_in(“www.bfe-networks.com|bfe-networks.com”)
    ```
  
  - **Note: the both sides of | can not be space**
  
    ```
    right：req_host_in(“www.bfe-networks.com|bfe-networks.com”)
    wrong：req_host_in(“www.bfe-networks.com | bfe-networks.com”)
    ```

## Path

- **req_path_in(patterns, case_insensitive)**
  
  - Judge if request path matches configured patterns
  
    ```
    // if path is /abc，case insensitive
    req_path_in(“/abc”, true)
    ```
- **req_path_prefix_in(patterns, case_insensitive)**
  - Judge if request path prefix matches configured patterns
  
    ```
    // if path prefix is /abc，case insensitive
    req_path_prefix_in(“/x/y”, false)
    ```
  
    
- **req_path_suffix_in(patterns, case_insensitive)**
  - Judge if request path suffix matches configured patterns
  
    ```
    // if path suffix is /abc，case insensitive
    req_path_suffix_in(“/x/y”, false)
    ```

**Note:**

**The patterns of req_path_in and req_path_prefix_in need to be included "/"**

## Query

- **req_query_key_in(patterns)**
  - Judge if query key matches configured patterns
  
    ```
    # if key in query is abc
    req_query_key_exist(“abc”)
    ```
- **req_query_key_prefix_in(patterns)**
  
  - Judge if query key prefix matches configured patterns
  
    ```
    # if key prefix in query is abc
    req_query_key_prefix_in(“abc”)
    ```
- **req_query_value_in(key, patterns, case_insensitive)**
  
  - Judge if value of query key matches configured patterns
  
    ```
    # if the value of abc in query is XXX, case insensitive
    req_query_value_in(“abc”, "XXX", true)
    ```
- **req_query_value_prefix_in(key, patterns, case_insensitive)**
  - Judge if value prefix of query key matches configured patterns
  
    ```
    # if the value prefix of abc in query is XXX, case insensitive
    req_query_value_prefix_in(“abc”, "XXX", true)
    ```
- **req_query_value_suffix_in(key, patterns, case_insensitive)**
  - Judge if value suffix of query key matches configured patterns
  
    ```
    # if the value suffix of abc in query is XXX, case insensitive
    req_query_value_suffix_in(“abc”, "XXX", true)
    ```
- **req_query_value_hash_in(key, patterns, case_insensitive)**
  - Judge if value of key after hash in request query matches configured patterns (value after hash is 0～9999)
  
    ```
    # if the value after hash of abc in query is 100, case insensitive
    req_query_value_hash_in(“abc”, "100", true)
    ```

## Port

- **req_port_in(patterns)**
  
  - Judge if port matches configured patterns
  
    ```
    # check if port is 80 or 8080
    req_port_in(“80|8080”)
    ```

## URL

- **req_url_regmatch(patterns)**
  - patterns is regular expression to match yrl
  
  - ` is recommended using
  
    ```
    # check if url is "/s?word=123"
    req_url_regmatch(`/s\?word=123`)
    ```
