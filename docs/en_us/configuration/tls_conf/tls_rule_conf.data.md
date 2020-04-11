# Introduction

tls_rule_conf.data records the tls protocol config

# Configuration

| Config Item       | Description                                                             |
| ----------------- | ----------------------------------------------------------------------- |
| Version           | String<br>Version of config file                                                  |
| DefaultNextProtos | String Array<br>Default application layer protocols over TLS                            |
| Config            | Struct<br>Tls rule config. Key: unique label, Value: tls rule detail              |
| CertName     | String<br>Name of server certificate (Defined in server_cert_conf.data)  |
| NextProtos   | String Array<br>TLS application layer protocol<br>- if empty, default http/1.1 |
| Grade        | String<br>TLS Security grade (A+, A, B, C)                           |
| ClientAuth   | Bool<br>Enable TLS Client Authentication                               |
| ClientCAName | String<br>Name of Client CA certificate                                  |
| VipConf      | String Array<br>List of VIP addresses                                          |
| SniConf      | String Array<br>List of hostnames (optional)                                   |

# Example

```
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
