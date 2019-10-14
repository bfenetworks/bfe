# IP

- **ses_sip_range("start_ip", "end_ip")**
  - Judge if srouce IP of session is in [start_ip, end_ip]
    ```
    # if source IP is in 10.0.0.1~10.0.0.10
    ses_sip_range("10.0.0.1", "10.0.0.10")
    ```

- **ses_vip_range("start_ip", "end_ip")**
  - Judge if VIP of session is in [start_ip, end_ip]
    ```
    # if vip is in 10.0.0.1~10.0.0.10
    ses_vip_range("10.0.0.1", "10.0.0.10")
    ```
