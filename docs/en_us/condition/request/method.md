# Method

- **req_method_in(patterns)**
  - Judge if request method matches configured patterns
  - **patterns** represent a set of patterns. The pattern should be GET/POST/PUT/DELETE.
  - **patterns** format: "pattern1|pattern2".
  ```
  # if the method of request is GET or POST
  req_method_in("GET|POST")
  ```
