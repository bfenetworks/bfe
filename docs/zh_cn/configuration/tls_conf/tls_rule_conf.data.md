# TLS协议配置

## 配置简介

tls_rule_conf.data配置TLS协议参数。

## 配置描述

| 配置项                 | 类型      | 参数含义                                 | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ---------------------- | --------- | ---------------------------------------- | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version                | String    | 配置文件版本                             | Y    | 参见 [Version](../00-common.md#5-配置文件版本version) 类型定义  | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| Config                 | Object    | 所有TLS协议配置                          | Y    | 键为标签，值为TLS协议配置详情                                | 非空                                                         |
| Config{k}              | String    | 标签                                     | Y    | 作为 Config 的键                                             | 非空                                                         |
| Config{v}              | Object    | TLS协议配置详情                          | Y    | 包含 CertName、NextProtos、Grade 等                          | 非空                                                         |
| Config{v}.CertName     | String    | 服务端证书名称                           | Y    | 须在 `server_cert_conf.data` 中定义                          | 非空                                                         |
| Config{v}.NextProtos   | Object    | TLS应用层协议列表                        | N    | 默认值为空，继承顶层 DefaultNextProtos                       | 元素须为支持的 TLS 应用层协议                                |
| Config{v}.NextProtos[] | String    | TLS应用层协议                            | Y    | 作为 NextProtos 的元素                                       | 合法值包括 `h2`、`spdy/3.1`、`http/1.1`、`stream`；支持参数化语法如 `proto;level=0;mcs=200;isw=65535;rate=100;pp=1` |
| Config{v}.Grade        | String    | TLS安全等级                              | N    | 未配置时默认值为 `C`                                         | 仅支持 `A+`、`A`、`B`、`C`                                   |
| Config{v}.ClientAuth   | Boolean   | 是否启用TLS双向认证                      | N    | 默认值为 `false`                                             | -                                                            |
| Config{v}.ClientCAName | String    | 客户端证书签发CA名称                     | 条件 | `ClientAuth=true` 时必填                                     | 非空；`ClientAuth=true` 时必须配置                          |
| Config{v}.Chacha20     | Boolean   | 是否优先使用ChaCha20加密套件             | N    | 默认继承 DefaultChacha20                                     | -                                                            |
| Config{v}.DynamicRecord | Boolean  | 是否开启动态TLS记录大小                  | N    | 默认继承 DefaultDynamicRecord                                | -                                                            |
| Config{v}.VipConf      | Object    | VIP列表                                  | N    | 优先依据VIP来确定TLS配置                                     | 元素须为有效 IP                                            |
| Config{v}.VipConf[]    | String    | VIP                                      | Y    | 作为 VipConf 的元素；参见 [IPAddr](../00-common.md#7-ip-地址ipaddr) 类型定义 | 类型为 [IPAddr](../00-common.md#7-ip-地址ipaddr)                |
| Config{v}.SniConf      | Object    | 域名列表                                 | N    | 无法依据VIP确定TLS配置时，使用SNI确定TLS配置                 | 元素须为有效域名                                           |
| Config{v}.SniConf[]    | String    | 域名                                     | Y    | 作为 SniConf 的元素                                          | 非空；须为有效域名或主机名                                   |
| DefaultNextProtos      | Object    | 支持的TLS应用层协议列表                  | N    | 默认值为 `["http/1.1"]`                                      | 元素须为支持的 TLS 应用层协议                                |
| DefaultNextProtos[]    | String    | TLS应用层协议                            | Y    | 作为 DefaultNextProtos 的元素                                | 合法值包括 `h2`、`spdy/3.1`、`http/1.1`、`stream`；支持参数化语法 |
| DefaultChacha20        | Boolean   | 全局默认是否优先使用ChaCha20加密套件     | N    | -                                                            | -                                                            |
| DefaultDynamicRecord   | Boolean   | 全局默认是否开启动态TLS记录大小          | N    | -                                                            | -                                                            |

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
| TLS1.2  | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |

### 安全等级A

| 支持协议 | 支持加密套件 |
| -------- | ------------ |
| TLS1.2<br>TLS1.1<br>TLS1.0 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |

### 安全等级B

| 支持协议 | 支持加密套件 |
| -------- | ------------ |
| TLS1.2<br>TLS1.1<br>TLS1.0 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |
| SSLv3 | TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_ECDHE_ECDSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_RC4_128_SHA |

**注：** SSLv3 下的 ECDSA RC4 套件仅在 `onlyRC4` 模式下生效。

### 安全等级C

| 支持协议 | 支持加密套件 |
| -------- | ------------ |
| TLS1.2<br>TLS1.1<br>TLS1.0 | TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA<br>TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_ECDHE_ECDSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_RC4_128_SHA |
| SSLv3 | TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_ECDHE_ECDSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_RC4_128_SHA |
