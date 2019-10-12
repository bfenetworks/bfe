# Response status code相关的条件原语

- **res_code_in(codes)**
  - 判断响应状态码是否为指定的任意状态码
  
  - codes代表多个状态码，是一个字符串，格式示例“200|400|403”
  
    ```
    # 响应返回状态码为200或500
    res_code_in(200|500)
    ```
