# Header相关

## 通用原语参数

- header_name：字符串，header中的key
- patterns：字符串，表示多个可匹配的pattern，用‘|’连接
- case_insensitive：bool类型，是否忽略key的值大小写

## 通用Header原语

- **req_header_key_in(patterns)**

  - 判断请求头部中key是否为patterns之一

  - **注意：Header首字母要大写**

    ```
  正确：req_header_key_in(“Header-Test”)
    错误：req_header_key_in(“Header-test”), req_header_key_in(“header-test”), req_header_key_in(“header-Test”)
    ```
  
- **req_header_value_in(header_name, patterns, case_insensitive)**
  
  - 判断http消息头部字段是否为patterns之一
  
    ```
    # header中Host为XXX.com的请求
    req_header_value_in("Host", "XXX.com", true)
    ```
- **req_header_value_prefix_in(header_name, patterns, case_insensitive)**
  - 判断http消息头部字段是否前缀匹配patterns之一
  
    ```
    # header中Host值前缀为XXX的请求
    req_header_prefix_value_in("Host", "XXX", true)
    ```
- **req_header_value_suffix_in(header_name, patterns, case_insensitive)**
  - 判断http消息头部字段是否后缀匹配patterns之一
  
    ```
    # header中Host值后缀为XXX的请求
    req_header_suffix_value_in("Host", "XXX", true)
    ```
- **req_header_value_hash_in(header_name, patterns, case_insensitive)**
  - 对http消息头部字段值哈希取模，判断是否匹配patterns之一（模值0～9999）
  
    ```
    # header中Host值hash取模后，值为100-200或400的请求
    req_header_value_hash_in("Host", "100-200|400", true)
    ```

