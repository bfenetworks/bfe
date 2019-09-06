# IP地址相关

## 请求clientip相关的条件原语

- **req_cip_range("startIP", "endIP")**
  
  - 判断请求的clientip是否在 [startIP, endIP] 的区间内
  
    ```
    # clientip在10.0.0.1~10.0.0.10的请求
    req_cip_range("10.0.0.1", "10.0.0.10")
    ```
- **req_cip_trusted()**
  
  - 判断clientip是否为trust ip
- **req_cip_hash_in(patterns)**
  
  - 对cip哈希取模，判断是否匹配patterns之一（模值0～9999）
  
  - patterns：字符串，表示多个可匹配的pattern，用‘|’连接
  
    ```
    # 对clientip哈希取模后，值为100~200的请求
    req_cip_hash_in("100-200")
    ```

## 请求vip相关的条件原语

- **req_vip_in("vip1|vip2|vip3")**
  
  - 判断访问VIP是否在指定VIP列表中 
  
    ```
    # vip 为10.0.0.1或10.0.0.2的请求
    req_vip_in("10.0.0.1|10.0.0.2")
    ```
- **req_vip_range("startVIP", "endVIP")**
  - 判断访问VIP是否在指定 [startIP, endIP] 的区间内
  
    ```
    # vip在10.0.0.1~10.0.0.10的请求
    req_vip_range("10.0.0.1", "10.0.0.10")
    ```
  
    
