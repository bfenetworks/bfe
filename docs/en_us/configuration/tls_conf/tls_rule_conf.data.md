# Configuration about TLS

## Introduction

tls_rule_conf.data records the tls protocol config

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | See [Version](../00-common.md#5-version) type definition | Type must be [Version](../00-common.md#5-version) |
| Config | Object | TLS rule config | Y | Keys are labels, values are TLS rule details | Non-empty |
| Config{k} | String | Unique label | Y | Key of Config | Non-empty |
| Config{v} | Object | TLS rule detail | Y | Contains CertName, NextProtos, Grade, etc. | Non-empty |
| Config{v}.CertName | String | Name of server certificate | Y | Must be defined in `server_cert_conf.data` | Non-empty |
| Config{v}.NextProtos | Object | TLS application layer protocol list | N | Defaults to empty; inherits top-level DefaultNextProtos | Elements must be supported TLS application layer protocols |
| Config{v}.NextProtos[] | String | TLS application layer protocol | Y | Element of NextProtos | Valid values include `h2`, `spdy/3.1`, `http/1.1`, `stream`; parameterized syntax such as `proto;level=0;mcs=200;isw=65535;rate=100;pp=1` is also supported |
| Config{v}.Grade | String | TLS security grade | N | Default value is `C` if not configured | Only supports `A+`, `A`, `B`, `C` |
| Config{v}.ClientAuth | Boolean | Enable TLS client authentication | N | When set to `true`, ClientCAName must also be configured | - |
| Config{v}.ClientCAName | String | Name of Client CA certificate | Conditional | Required when `ClientAuth=true` | Non-empty; must be configured when `ClientAuth=true` |
| Config{v}.Chacha20 | Boolean | Prefer ChaCha20 cipher suites | N | Defaults to inherit DefaultChacha20 | - |
| Config{v}.DynamicRecord | Boolean | Enable dynamic TLS record size | N | Defaults to inherit DefaultDynamicRecord | - |
| Config{v}.VipConf | Object | List of VIPs | N | TLS policy selection is based on VIP | Elements must be valid IPs |
| Config{v}.VipConf[] | String | VIP | Y | Element of VipConf; see [IPAddr](../00-common.md#7-ipaddr) type definition | Type must be [IPAddr](../00-common.md#7-ipaddr) |
| Config{v}.SniConf | Object | List of hostnames | N | Used to determine TLS config when VIP cannot be used | Elements must be valid hostnames |
| Config{v}.SniConf[] | String | Hostname | Y | Element of SniConf | Non-empty; must be a valid hostname |
| DefaultNextProtos | Object | Default application layer protocols over TLS | N | Default value is `["http/1.1"]` | Elements must be supported TLS application layer protocols |
| DefaultNextProtos[] | String | TLS application layer protocol | Y | Element of DefaultNextProtos | Valid values include `h2`, `spdy/3.1`, `http/1.1`, `stream`; parameterized syntax is also supported |
| DefaultChacha20 | Boolean | Global default for preferring ChaCha20 cipher suites | N | - | - |
| DefaultDynamicRecord | Boolean | Global default for enabling dynamic TLS record size | N | - | - |

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
            "Grade": "B",
            "ClientCAName": ""
        }
    }
}
```

## Security Grade

BFE supports multiple security grades(A+/A/B/C) for ease of TLS configuration. Security grades vary depending on the protocols and the cipher suites supported. Grade A+ has the highest security and lowest connectivity; Grade C has the lowest security and highest connectivity.

### Grade A+

| Supported Protocols | Supported Cipher Suites |
| ------------------- | ----------------------- |
| TLS1.2 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |

### Grade A

| Supported Protocols | Supported Cipher Suites |
| ------------------- | ----------------------- |
| TLS1.2<br>TLS1.1<br>TLS1.0 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |

### Grade B

| Supported Protocols | Supported Cipher Suites |
| ------------------- | ----------------------- |
| TLS1.2<br>TLS1.1<br>TLS1.0 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |
| SSLv3 | TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_ECDHE_ECDSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_RC4_128_SHA |

**Note:** The ECDSA RC4 suite under SSLv3 is available only in `onlyRC4` mode.

### Grade C

| Supported Protocols | Supported Cipher Suites |
| ------------------- | ----------------------- |
| TLS1.2<br>TLS1.1<br>TLS1.0 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA<br>TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_ECDHE_ECDSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_RC4_128_SHA |
| SSLv3 | TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_ECDHE_ECDSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_RC4_128_SHA |
