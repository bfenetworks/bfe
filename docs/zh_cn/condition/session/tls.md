# TLS相关

## 通用原语参数
- patterns：字符串，表示多个可匹配的pattern，用‘|’连接

## TLS原语
- **ses_tls_sni_in(patterns)**
  - 判断TLS握手中的sni是否为patterns之一
  ```
  # TLS握手中的sni为www.bfe-networks.com或bfe-networks.com的会话
  ses_tls_sni_in("www.bfe-networks.com|bfe-networks.com")
  ```

- **ses_tls_client_auth()**
  - 判断是否启用TLS双向认证

- **ses_tls_client_ca_in(patterns)**
  - 判断是否启用TLS双向认证且客户端证书签发根CA为patterns之一
  ```
  # 启用TLS双向认证且客户端证书签发CA是ca1或ca2的会话
  ses_tls_client_ca_in("ca1|ca2")
  ```
