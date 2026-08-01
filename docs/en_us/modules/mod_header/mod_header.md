# mod_header

## Introduction

mod_header modifies header of HTTP request/response based on defined rules.

## Module Configuration

For details, see [mod_header.conf](../../configuration/mod_header/mod_header.conf.md).

## Rule Configuration

For details, see [mod_header.data](../../configuration/mod_header/mod_header.data.md).

## Builtin Variables

BFE provides a list of variables which are evaluated in the runtime during the processing of each request.
See the configuration example in [mod_header.data](../../configuration/mod_header/mod_header.data.md).

| Variable       | Description |
| -------------- | ----------- |
| %bfe_client_ip | Client IP |
| %bfe_client_port | Client port |
| %bfe_request_host | Value of Request Host header |
| %bfe_session_id | Session ID |
| %bfe_log_id | Request ID |
| %bfe_cip | Client IP (CIP) |
| %bfe_bip | Backend instance IP |
| %bfe_rip | BFE instance IP |
| %bfe_vip | Virtual IP (VIP) |
| %bfe_server_name | BFE instance address |
| %bfe_cluster | Backend cluster |
| %bfe_backend_info | Backend information |
| %bfe_ssl_resume | Whether the TLS/SSL session is resumed with session id or session ticket |
| %bfe_ssl_cipher | TLS/SSL cipher suite |
| %bfe_ssl_version | TLS/SSL version |
| %bfe_ssl_ja3_raw | JA3 fingerprint string for TLS/SSL client |
| %bfe_ssl_ja3_hash | JA3 fingerprint hash for TLS/SSL client |
| %bfe_http2_fingerprint | HTTP/2 fingerprint |
| %bfe_protocol | Application level protocol |
| %bfe_client_geo_country_iso_code | Client geo country ISO code |
| %bfe_client_geo_subdivision_iso_code | Client geo subdivision ISO code |
| %bfe_client_geo_city_name | Client geo city name |
| %bfe_client_geo_latitude | Client geo latitude |
| %bfe_client_geo_longitude | Client geo longitude |
| %client_cert_serial_number | Serial number of client certificate |
| %client_cert_subject_title | Subject title of client certificate |
| %client_cert_subject_common_name | Subject Common Name of client certificate|
| %client_cert_subject_organization | Subject Organization of client certificate |
| %client_cert_subject_organizational_unit | Subject Organizational Unit of client certificate |
| %client_cert_subject_province | Subject Province of client certificate |
| %client_cert_subject_country | Subject Country of client certificate |
| %client_cert_subject_locality | Subject Locality of client certificate |
