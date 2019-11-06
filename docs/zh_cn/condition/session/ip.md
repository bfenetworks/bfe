# IP地址相关

- **ses_sip_range("start_ip", "end_ip")**
  - 判断会话的源ip是否在 [start_ip, end_ip] 的区间内
  ```
  # clientip在10.0.0.1~10.0.0.10的会话
  ses_sip_range("10.0.0.1", "10.0.0.10")
  ```

- **ses_vip_range("startVIP", "end_ip")**
  - 判断访问VIP是否在指定 [start_ip, end_ip] 的区间内
  ```
  # vip在10.0.0.1~10.0.0.10的会话
  ses_vip_range("10.0.0.1", "10.0.0.10")
  ```

