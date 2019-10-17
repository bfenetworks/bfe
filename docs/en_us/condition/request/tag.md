# Tag

- **req_tag_match(tagName, tagValue)**
  - Judge if request tag matches configured value
  ```
  # if clientIP tag is blacklist
  req_tag_match("clientIP", "blacklist")
  ```
