# Configration about TLS

## Introduction

tls_rule_conf.data records the tls protocol config

## Configuration

| Config Item             | Description                                                                    |
| ----------------------- | ------------------------------------------------------------------------------ |
| Version                 | String<br>Version of configure file                                            |
| Config                  | Object<br>TLS rule config.                                                     |
| Config{k}               | String<br>Unique label                                                         |
| Config{v}               | Object<br>TLS rule detail                                                      |
| Config{v}.CertName      | String<br>Name of server certificate (Note: defined in server_cert_conf.data)  |
| Config{v}.NextProtos    | Object<br>TLS application layer protocol list <br>Default ["http/1.1"]         |
| Config{v}.NextProtos[]  | String<br>TLS application layer protocol (h2, spdy/3.1, http/1.1)              |
| Config{v}.Grade         | String<br>TLS Security grade ( A+, A, B, C)                                    |
| Config{v}.ClientAuth    | Bool<br>Enable TLS Client Authentication                                       |
| Config{v}.ClientCAName  | String<br>Name of Client CA certificate                                        |
| Config{v}.VipConf       | Object Array<br>List of VIPs<br>Note: TLS policy selection is based on VIP     |
| Config{v}.VipConf[]     | String Array<br>VIP                                                            |
| Config{v}.SniConf       | Object Array<br>List of hostnames (optional)                                   |
| Config{v}.SniConf[]     | String Array<br>Hostname                                                       |
| DefaultNextProtos       | Object<br>Default application layer protocols over TLS                         |
| DefaultNextProtos[]     | String<br>TLS application layer protocol (h2, spdy/3.1, http/1.1)              |

## Example

```json
{
    "Version": "20190101000000",
    "DefaultNextProtos": ["h2", "http/1.1"],
    "Config": {
        "example_product": {
            "VipConf": [
                "10.199.4.14"
            ],
            "SniConf": null,
            "CertName": "example.org",
            "NextProtos": [
                "h2",
                "http/1.1"
            ],
            "Grade": "C",
            "ClientCAName": ""
        }
    }
}
```

## Security Grade

BFE supports multiple security grades(A+/A/B/C) for ease of TLS configuration. Security grades vary depending on the protocols and the cipher suites supported.

### Grade A+

| Supported Protocols | Supported Cipher Suites |
| ------------------- | ----------------------- |
| TLS1.2              | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_OLD_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |

### Grade A

| Supported Protocols | Supported Cipher Suites |
| ------------------- | ----------------------- |
| TLS1.2<br>TLS1.1<br>TLS1.0 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_OLD_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |

### Grade B

| Supported Protocols | Supported Cipher Suites |
| ------------------- | ----------------------- |
| TLS1.2<br>TLS1.1<br>TLS1.0 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_OLD_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |
| SSLv3 | TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_RC4_128_SHA |

### Grade C

| Supported Protocols | Supported Cipher Suites |
| ------------------- | ----------------------- |
| TLS1.2<br>TLS1.1<br>TLS1.0 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_OLD_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA<br>TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_RC4_128_SHA |
| SSLv3 | TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_RC4_128_SHA |
