# Header相关

## 通用Header原语

- **req_header_key_in(patterns)**

  - 判断请求头部中key是否为patterns之一

  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接

    **注意：**

    ```
    Header首字母要大写
    正确：req_header_key_in(“Header-Test”)
    错误：req_header_key_in(“Header-test”), req_header_key_in(“header-test”), req_header_key_in(“header-Test”)
    ```

- **req_header_value_in(header_name, patterns, case_insensitive)**
  - 判断http消息头部字段是否为patterns之一
  - header_name，字符串，头部字段名称
  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接
  - case_insensitive，bool类型，是否忽略大小写
- **req_header_value_prefix_in(header_name, patterns, case_insensitive)**
  - 判断http消息头部字段是否前缀匹配patterns之一
  - header_name，字符串，头部字段名称
  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接
  - case_insensitive，bool类型，是否忽略大小写
- **req_header_value_suffix_in(header_name, patterns, case_insensitive)**
  - 判断http消息头部字段是否后缀匹配patterns之一
  - header_name，字符串，头部字段名称
  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接
  - case_insensitive，bool类型，是否忽略大小写
- **req_header_value_hash_in(header_name, patterns, case_insensitive)**
  - 对http消息头部字段值哈希取模，判断是否匹配patterns之一（模值0～9999）
  - header_name，字符串，头部字段名称
  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接（如 “0-100|1000-1100|9999”）
    - 单值形式：100
    - 多值形式：100|101|102
    - 范围形式：100-200
    - 混合形式：100-200|400-500
  - case_insensitive，bool类型，是否忽略头部字段值大小写