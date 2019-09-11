# IP

## Condition Primitive About ClientIP

- **req_cip_range("startIP", "endIP")**
  - Judge if client IP is in [startIP, endIP]
  
    ```
    # if client IP is in 10.0.0.1~10.0.0.10
    req_cip_range("10.0.0.1", "10.0.0.10")
    ```
- **req_cip_trusted()**
  
  - Judge if client IP is trust IP
- **req_cip_hash_in(patterns)**
  - Judge if client IP after hash matches configured patterns (value after hash is 0～9999)
  
  - Patterns represent multiple patterns. The type of it is string, format is as "pattern1|pattern2" (e.g. “0-100|1000-1100|9999”)
  
    ```
    # if the value after hash of client IP is 100~200
    req_cip_hash_in("100-200")
    ```

## Condition Primitive About VIP

- **req_vip_in("vip1|vip2|vip3")**
  - Judge if VIP is in configured VIP list
  
    ```
    # if vip is 10.0.0.1 or 10.0.0.2
    req_vip_in("10.0.0.1|10.0.0.2")
    ```
- **req_vip_range("startVIP", "endVIP")**
  - Judge if VIP is in [startIP, endIP]
  
    ```
    # if vip is in 10.0.0.1~10.0.0.10
    req_vip_range("10.0.0.1", "10.0.0.10")
    ```
  
    
