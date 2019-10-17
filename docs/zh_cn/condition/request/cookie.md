# 请求Cookie相关

## 通用原语参数
- key：字符串，cookie中的key
- patterns：字符串，表示多个可匹配的pattern，用‘|’连接
- case_insensitive：bool类型，是否忽略大小写

## 请求Cookie原语
- **req_cookie_key_in(patterns)**
  - 判断Cookie key是否为patterns之一
  ```
  # 判断请求cookie key为UID1或UID2或UID3
  req_cookie_key_in(“UID1|UID2|UID3”)
  ```

- **req_cookie_value_in(key, patterns, case_insensitive)**
  - 判断cookie中key对应的值是否为patterns之一
  ```
  # 判断请求cookie中UID的值(忽略大小写)是否是XXX
  req_cookie_value_in(“UID”, "XXX", true)
  ```

- **req_cookie_value_prefix_in(key, patterns, case_insensitive)**
  - 判断cookie中key的值是否前缀匹配patterns之一
  ```
  # 判断请求cookie中UID的cookie值(忽略大小写)是否以XXX开头
  req_cookie_value_prefix_in(“UID”, "XXX", true)
  ```

- **req_cookie_value_suffix_in(key, patterns, case_insensitive)**
  - 判断cookie中key的值是否后缀匹配patterns之一
  ```
  # 判断请求cookie中UID的cookie值(忽略大小写)是否以XXX结尾
  req_cookie_value_suffix_in(“UID”, "XXX", true)
  ```

- **req_cookie_value_hash_in(key, patterns, case_insensitive)**
  - 对cookie中key的值哈希取模，判断是否匹配patterns之一（模值0～9999）
  ```
  # 判断请求cookie中UID的cookie值(忽略大小写)取模后是否为100
  req_cookie_value_hash_in(“UID”, “100”, true)
  ```

- **req_cookie_value_contain(key, patterns, case_insensitive)**
  - 判断cookie中key的值是否包含patterns之一
  ```
  # 判断请求cookie中UID的cookie值(忽略大小写)是否包含XXX
  req_cookie_value_contain(“UID”, “XXX”, true)
  ```

