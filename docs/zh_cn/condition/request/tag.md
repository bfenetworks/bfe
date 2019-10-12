# 请求Tag相关

- **req_tag_match(tagName, tagValue)**
  - 在请求处理过程中，在某些环节可能会打上一些标签
  - 如：在经过词典匹配后，设置clientIP这个tag的值为news_blackIPList
    
  ```
  # clientIP tag的值为news_blackIPList的请求
  req_tag_match("clientIP", "news_blackIPList")
  ```

