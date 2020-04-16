# Introduction

tls_rule_conf.data records the tls protocol config

# Configuration

| Config Item             | Description                                                                    |
| ----------------------- | ------------------------------------------------------------------------------ |
| Version                 | String<br>Version of configure file                                            |
| Config                  | Object<br>TLS rule config.                                                     |
| Config.{k}              | String<br>Unique label                                                         |
| Config.{v}              | Object<br>TLS rule detail                                                      |
| Config.{v}.CertName     | String<br>Name of server certificate (Note: defined in server_cert_conf.data)  |
| Config.{v}.NextProtos   | Object<br>TLS application layer protocol list <br>- Default is ["http/1.1"]    |
| Config.{v}.NextProtos[] | String<br>TLS application layer protocol<br>- Contains h2, spdy/3.1, http/1.1  |
| Config.{v}.Grade        | String<br>TLS Security grade, Contains A+, A, B, C                             |
| Config.{v}.ClientAuth   | Bool<br>Enable TLS Client Authentication                                       |
| Config.{v}.ClientCAName | String<br>Name of Client CA certificate                                        |
| Config.{v}.VipConf      | Object Array<br>List of VIP addresses (Note: priority is given to TLS configuration based on VIP)  |
| Config.{v}.VipConf[]    | String Array<br>VIP                                                            |
| Config.{v}.SniConf      | Object Array<br>List of hostnames (optional) <br>- (Note: when TLS configuration cannot be determined according to VIP, SNI is used to determine TLS configuration)  |
| Config.{v}.SniConf[]    | String Array<br>Hostname                                                       |
| DefaultNextProtos       | Object<br>Default(Supported) application layer protocols over TLS              |
| DefaultNextProtos[]     | String<br>TLS application layer protocol<br>- Contains h2, spdy/3.1, http/1.1  |

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
