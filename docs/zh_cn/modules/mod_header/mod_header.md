# mod_header

## 模块简介

mod_header根据自定义条件，修改请求或响应的头部。

## 基础配置

模块基础配置文件说明详见 [mod_header.conf](../../configuration/mod_header/mod_header.conf.md)。

## 规则配置

模块规则配置文件说明详见 [mod_header.data](../../configuration/mod_header/mod_header.data.md)。

## 内置变量说明

BFE支持如下一系列变量并在处理请求阶段求值。关于变量的使用参见 [mod_header.data](../../configuration/mod_header/mod_header.data.md) 配置示例。

| 变量名         | 含义       |
| -------------- | ---------- |
| %bfe_client_ip | 客户端IP |
| %bfe_client_port | 客户端端口 |
| %bfe_request_host | 请求Host |
| %bfe_session_id | 会话ID |
| %bfe_log_id | 请求ID |
| %bfe_cip | 客户端IP (CIP) |
| %bfe_bip | 后端实例IP |
| %bfe_rip | BFE实例IP |
| %bfe_vip | 服务端IP (VIP) |
| %bfe_server_name | BFE实例地址 |
| %bfe_cluster | 目的后端集群 |
| %bfe_backend_info | 后端信息 |
| %bfe_ssl_resume | 是否TLS/SSL会话复用 |
| %bfe_ssl_cipher | TLS/SSL加密套件 |
| %bfe_ssl_version | TLS/SSL协议版本 |
| %bfe_ssl_ja3_raw | TLS/SSL客户端JA3算法指纹数据 |
| %bfe_ssl_ja3_hash | TLS/SSL客户端JA3算法指纹哈希值 |
| %bfe_http2_fingerprint | HTTP/2 指纹 |
| %bfe_protocol | 访问协议 |
| %bfe_client_geo_country_iso_code | 客户端地理国家 ISO 代码 |
| %bfe_client_geo_subdivision_iso_code | 客户端地理行政区划 ISO 代码 |
| %bfe_client_geo_city_name | 客户端地理城市名 |
| %bfe_client_geo_latitude | 客户端地理纬度 |
| %bfe_client_geo_longitude | 客户端地理经度 |
| %client_cert_serial_number | 客户端证书序列号 |
| %client_cert_subject_title | 客户端证书Subject title |
| %client_cert_subject_common_name | 客户端证书Subject Common Name |
| %client_cert_subject_organization | 客户端证书Subject Organization |
| %client_cert_subject_organizational_unit | 客户端证书Subject Organizational Unit |
| %client_cert_subject_province | 客户端证书Subject Province |
| %client_cert_subject_country | 客户端证书Subject Country |
| %client_cert_subject_locality | 客户端证书Subject Locality |
