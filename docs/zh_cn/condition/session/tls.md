# TLS相关

## 通用原语参数
- patterns：字符串，表示多个可匹配的pattern，用‘|’连接

## TLS原语
- **ses_tls_sni_in(patterns)**
  - 判断TLS握手中的sni是否为patterns之一
  ```
  # TLS握手中的sni为www.bfe-networks.com或bfe-networks.com的会话
  ses_tls_sni_in(“www.bfe-networks.com|bfe-networks.com”)
  ```

- **ses_tls_client_auth()**
  - 判断TLS握手是否启用了双向认证

- **ses_tls_client_ca_in(patterns)**
  - 判断TLS握手使用的证书名称是否为patterns之一
  ```
  # TLS握手是用的证书名称为bfe-networks.com或net的会话
  ses_tls_client_ca_in(“bfe-networks.com|net”)
  ```