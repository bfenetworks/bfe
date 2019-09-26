# Tag

- **req_tag_match(tagName, tagValue)**
  - In process of request, tag can be added sometimes
    
    - e.g. After matching dictionary, set value of clientIP(tag) to  news_blackIPList
    
    ```
    # if the value of clientIP tag is news_blackIPList
    req_tag_match("clientIP", "news_blackIPList")
    ```
    
    
