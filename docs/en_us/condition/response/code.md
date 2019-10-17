# Response Status Code

- **res_code_in(codes)**
  - Judge response HTTP status code is in configured codes
  - Codes represent multiple HTTP status code, it is string, format is as "200|400|403"
  ```
  # if response status code is 200 or 500
  res_code_in(200|500)
  ```
