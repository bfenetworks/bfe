# mod_header

## Introduction

mod_header modifies header of HTTP request/response based on defined rules.

## Module Configuration

### Description

conf/mod_header/mod_header.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.DataPath | String<br>Path of rule configuration |
| Log.OpenDebug | Boolean<br>Debug flag of module |

### Example

```ini
[Basic]
DataPath = mod_header/header_rule.data
```

## Rule Configuration

### Description

conf/mod_header/header_rule.data

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of config file |
| Config      | Struct<br>Header rules for each product |
| Config{k}   | String<br>Product name |
| Config{v}   | Object<br>A ordered list of rules |
| Config{v}[] | Object<br>A rule |
| Config{v}[].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].Last | Boolean<br>If true, stop processing the next rule |
| Config{v}[].Actions | Object<br>A list of Actions |
| Config{v}[].Actions.Cmd | String<br>A Action |
| Config{v}[].Actions.Params | Object<br>A list of parameters for action |
| Config{v}[].Actions.Params[] | String<br>A parameter |

### Actions

| Action         | Description            | Parameters |
| -------------- | ---------------------- | ---------- |
| REQ_HEADER_SET | Set request header     | HeaderName, HeaderValue |
| REQ_HEADER_ADD | Add request header     | HeaderName, HeaderValue |
| REQ_HEADER_DEL | Delete request header  | HeaderName |
| RSP_HEADER_SET | Set response header    | HeaderName, HeaderValue |
| RSP_HEADER_ADD | Add response header    | HeaderName, HeaderValue |
| RSP_HEADER_DEL | Delete response header | HeaderName |

### Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "cond": "req_path_prefix_in(\"/header\", false)",
                "actions": [
                    {
                        "cmd": "REQ_HEADER_SET",
                        "params": [
                            "X-Bfe-Log-Id",
                            "%bfe_log_id"
                        ]
                    },
                    {
                        "cmd": "REQ_HEADER_SET",
                        "params": [
                            "X-Bfe-Vip",
                            "%bfe_vip"
                        ]
                    },
                    {
                        "cmd": "RSP_HEADER_SET",
                        "params": [
                            "X-Proxied-By",
                            "bfe"
                        ]
                    }
                ],
                "last": true
            }
        ]
    }
}
```

## Builtin Variables

BFE provides a list of variables which are evaluated in the runtime during the processing of each request.
See the **Example** above.

| Variable       | Description |
| -------------- | ----------- |
| %bfe_client_ip | Client IP |
| %bfe_client_port | Client port |
| %bfe_request_host | Value of Request Host header |
| %bfe_session_id | Session ID |
| %bfe_log_id | Request ID |
| %bfe_cip | Client IP (CIP) |
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
| %client_cert_serial_number | Serial number of client certificate |
| %client_cert_subject_title | Subject title of client certificate |
| %client_cert_subject_common_name | Subject Common Name of client certificate|
| %client_cert_subject_organization | Subject Organization of client certificate |
| %client_cert_subject_organizational_unit | Subject Organizational Unit of client certificate |
| %client_cert_subject_province | Subject Province of client certificate |
| %client_cert_subject_country | Subject Country of client certificate |
| %client_cert_subject_locality | Subject Locality of client certificate |
