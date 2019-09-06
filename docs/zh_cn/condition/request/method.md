# method相关

- **req_method_in(patterns)**
  
  - 请求方法是否匹配patterns之一
  
  - Patterns，字符串，表示多个可匹配的pattern，用‘|’连接，pattern取值只能是GET/POST/PUT/DELETE
  
    ```
    # 使用GET或POST方法的请求
    req_method_in("GET|POST")
    ```