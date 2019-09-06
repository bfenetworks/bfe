# Method

- **req_method_in(patterns)**
  - Judge if request method matches configured patterns
  
  - Patterns represent multiple patterns. The type of it is string, format is as "pattern1|pattern2". The value of pattern only can be GET/POST/PUT/DELETE
  
    ```
    # if the method of request is GET or POST
    req_method_in("GET|POST")
    ```
  
    

