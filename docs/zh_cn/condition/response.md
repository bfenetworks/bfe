# Response相关的条件原语

## 1. status code相关

- **res_code_in(codes)**
  - 判断响应状态码是否为指定的任意状态码
  - codes代表多个状态码，是一个字符串，格式示例“200|400|403”

## 2. header相关

- **res_header_key_in(patterns)**
  - 判断响应头部中key是否满足patterns之一
  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接
- **res_header_value_in(key, patterns, case_insensitive)**
  - 判断header中key对应的值是否满足patterns之一
  - key，字符串
  - patterns，字符串，表示多个可匹配的pattern，用‘|’连接