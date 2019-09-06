# URI相关

URI的一般形式：

- http://host[:port\]/path/?query](http://host/path/?query)

## 通用原语参数

- patterns：字符串，表示多个可匹配的pattern，用‘|’连接

- case_insensitive：bool类型，是否忽略key的值大小写

## Host相关

以下列举了判断http request中host相关的条件原语，由于host本身是忽略大小写的，所以所有host相关的原语不提供是否大小写忽略选项

- **req_host_in(patterns)**
  - 判断http的host是否为patterns之一
  
  - 忽略大小写精确匹配
  
    ```
    // 匹配www.bfe-networks.com或bfe-networks.com，忽略大小写
    req_host_in(“www.bfe-networks.com|bfe-networks.com”)
    ```
  
  - **注意: 多个pattern中间的|两侧不可以有空格**
  
    ```
    正确：req_host_in(“www.bfe-networks.com|bfe-networks.com”)
    错误：req_host_in(“www.bfe-networks.com | bfe-networks.com”)
    ```

## Path相关

判断url中path部分的条件原语

- **req_path_in(patterns, case_insensitive)**
  - 判断http的path是否为patterns之一
  
    ```
    // path是否为/abc，忽略大小写
    req_path_in(“/abc”, true)
    ```
- **req_path_prefix_in(patterns, case_insensitive)**
  - 判断http的path是否前缀匹配patterns之一
  
    ```
    // path的前缀是否为/x/y，大小写敏感
    req_path_prefix_in(“/x/y”, false)
    ```
- **req_path_suffix_in(patterns, case_insensitive)**
  - 判断http的path是否后缀匹配patterns之一
  
    ```
    // path的后缀是否为/x/y，大小写敏感
    req_path_suffix_in(“/x/y”, false)
    ```

**注意：**

**req_path_in和req_path_prefix_in的patterns需要包含开头的/**

## Query相关

查询参数相关的条件原语

- **req_query_key_in(patterns)**
  
  - 判断query key是否为patterns之一
  
    ```
    # 查询参数中是否有名字为abc的key
    req_query_key_exist(“abc”)
    ```
- **req_query_key_prefix_in(patterns)**
  
  - 判断query key是否为前缀匹配patterns之一
  
    ```
    # 查询参数中是否有名字前缀为abc的key
    req_query_key_prefix_in(“abc”)
    ```
- **req_query_value_in(key, patterns, case_insensitive)**
  
  - 判断query中key的值是否精确匹配patterns之一
  
    ```
    # 查询参数中key为abc的参数值为XXX的请求，大小写忽略
    req_query_value_in(“abc”, "XXX", true)
    ```
- **req_query_value_prefix_in(key, patterns, case_insensitive)**
  
  - 判断query中key的值是否前缀匹配patterns之一
  
    ```
    # 查询参数中key为abc的参数值前缀为XXX的请求，大小写忽略
    req_query_value_prefix_in(“abc”, "XXX", true)
    ```
- **req_query_value_suffix_in(key, patterns, case_insensitive)**
  - 判断query中key的值是否后缀匹配patterns之一
  
    ```
    # 查询参数中key为abc的参数值前缀为XXX的请求，大小写忽略
    req_query_value_suffix_in(“abc”, "XXX", true)
    ```
- **req_query_value_hash_in(key, patterns, case_insensitive)**
  
  - 对query中key的值哈希取模，判断是否匹配patterns之一（模值0～9999）
  
    ```
    # 查询参数中key为abc的参数值哈希取模后值为100的请求，大小写忽略
    req_query_value_hash_in(“abc”, "100", true)
    ```

## Port相关

查询参数相关的条件原语

- **req_port_in(patterns)**
  
  - 判断请求端口是否为patterns之一
  
    ```
    # 查询端口是否为80或8080
    req_port_in(“80|8080”)
    ```

## 完整URL相关

查询参数相关的条件原语

- **req_url_regmatch(patterns)**
  - patterns，正则表达式，用来匹配完整url的正则表达式
  
  - 推荐使用反引号，不需要额外进行转义
  
    ```
    # 查询url是否是/s?word=123
    req_url_regmatch(`/s\?word=123`)
    ```
