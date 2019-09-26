# Introduction

tls_rule_conf.data records the tls protocol config

# Configuration

| Config Item       | Type         | Description                                                             |
| ----------------- | ------------ | ----------------------------------------------------------------------- |
| Version           | String       | Version of config file                                                  |
| DefaultNextProtos | String Array | Default application layer protocols over TLS                            |
| Config            | Struct       | Tls rule config. Key: unique label, Value: tls rule detail              |

The struct of tls rule detail is as followed: 

| Config Item  | Type         | Description                                                    |
| ------------ | ------------ | -------------------------------------------------------------- |
| CertName     | String       | Name of server certificate (Defined in server_cert_conf.data)  |
| NextProtos   | String Array | TLS application layer protocol<br>- if empty, default http/1.1 |
| Grade        | String       | TLS Security grade (A+, A, B, C)                           |
| ClientAuth   | Bool         | Enable TLS Client Authentication                               |
| ClientCAName | String       | Name of Client CA certificate                                  |
| VipConf      | String Array | List of VIP addresses                                          |
| SniConf      | String Array | List of hostnames (optional)                                   |

# Example

```
{
    "Version": "20190101000000",
    "DefaultNextProtos": ["http/1.1"],
    "Config": {
        "example_product": {
            "VipConf": [
                "10.199.4.14"
            ],
            "SniConf": null,
            "CertName": "example.org",
            "NextProtos": [
                "h2;rate=0;isw=65535;mcs=200;level=0",
                "http/1.1"
            ],
            "Grade": "C",
            "ClientCAName": ""
        }
    }
}
```
