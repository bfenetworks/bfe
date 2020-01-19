# 请求method相关

- **req_method_in(patterns)**
  - 请求方法是否匹配patterns之一
  - Patterns，字符串，表示一个或多个用‘|’连接的pattern，pattern取值只能是GET/POST/PUT/DELETE
  ```
  # 判断请求method是否是GET或POST方法
  req_method_in("GET|POST")
  ```
