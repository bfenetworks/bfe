## Cookie相关

## 通用Cookie原语

- **req_cookie_key_in(patterns)**
  - 判断Cookie key是否为patterns之一
  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接
- **req_cookie_value_in(key, patterns, case_insensitive)**
  - 判断cookie中key对应的值是否为patterns之一
  - key，字符串
  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接
- **req_cookie_value_prefix_in(key, patterns, case_insensitive)**
  - 判断cookie中key的值是否前缀匹配patterns之一
  - key，字符串
  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接
- **req_cookie_value_suffix_in(key, patterns, case_insensitive)**
  - 判断cookie中key的值是否后缀匹配patterns之一
  - key，字符串
  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接
- **req_cookie_value_hash_in(key, patterns, case_insensitive)**
  - 对cookie中key的值哈希取模，判断是否匹配patterns之一（模值0～9999）
  - key，字符串
  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接（如 “0-100|1000-1100|9999”）
  - case_insensitive，bool类型，是否忽略key的值大小写

举例：

```
#UID的cookie值是否以XXX结尾
req_cookie_value_suffix_in(“UID”, “XXX”)
```

