# TLS协议配置

## 配置简介

tls_rule_conf.data配置TLS协议参数。

## 配置描述

| 配置项                 | 描述                                                          |
| ---------------------- | ------------------------------------------------------------- |
| Version                | String<br>配置文件版本                                        |
| Config                 | Object<br>所有TLS协议配置                                     |
| Config{k}              | String<br>标签                                                |
| Config{v}              | Object<br>TLS协议配置详情                                     |
| Config{v}.CertName     | String<br>服务端证书名称（注：在server_cert_conf.data文件中定义）|
| Config{v}.NextProtos   | Object<br>TLS应用层协议列表<br>默认值为["http/1.1"]               |
| Config{v}.NextProtos[] | String<br>TLS应用层协议, 合法值包括h2, spdy/3.1, http/1.1     |
| Config{v}.Grade        | String<br>TLS安全等级, 合法值包括A+，A，B，C                  |
| Config{v}.ClientAuth   | Bool<br>是否启用TLS双向认证                                   |
| Config{v}.ClientCAName | String<br>客户端证书签发CA名称                                |
| Config{v}.VipConf      | Object<br>VIP列表（注：优先依据VIP来确定TLS配置）             |
| Config{v}.VipConf[]    | String<br>VIP                                                 |
| Config{v}.SniConf      | Object<br>域名列表，可选（注：无法依据VIP确定TLS配置时，使用SNI确定TLS配置）|
| Config{v}.SniConf[]    | String<br>域名                                                |
| DefaultNextProtos      | Object<br>支持的TLS应用层协议列表                             |
| DefaultNextProtos[]    | String<br>TLS应用层协议, 合法值包括h2, spdy/3.1, http/1.1     |

## 配置示例

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

## 安全等级说明

BFE支持多种安全等级（A+/A/B/C）。各安全等级差异在于支持的协议版本及加密套件。A+等级安全性最高、连通性最低；C等级安全性最低、连通性最高。

### 安全等级A+

| 支持协议 | 支持加密套件 |
| -------- | ------------ |
| TLS1.2  | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_OLD_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |

### 安全等级A

| 支持协议 | 支持加密套件 |
| -------- | ------------ |
| TLS1.2<br>TLS1.1<br>TLS1.0 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_OLD_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |

### 安全等级B

| 支持协议 | 支持加密套件 |
| -------- | ------------ |
| TLS1.2<br>TLS1.1<br>TLS1.0 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_OLD_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |
| SSLv3 | TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_RC4_128_SHA |

### 安全等级C

| 支持协议 | 支持加密套件 |
| -------- | ------------ |
| TLS1.2<br>TLS1.1<br>TLS1.0 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_OLD_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA<br>TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_RC4_128_SHA |
| SSLv3 | TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_RC4_128_SHA |
