# 简介

tls_rule_conf.data配置TLS协议参数。

# 配置

| 配置项            | 类型         | 描述                                                          |
| ----------------- | ------------ | ------------------------------------------------------------- |
| Version           | String       | 配置文件版本                                                  |
| DefaultNextProtos | String Array | 默认的应用层协议(h2, spdy/3.1, http/1.1)                      |
| Config            | Struct       | TLS配置，该配置为map数据，key是产品线名称，value是tls配置详情 |

TLS配置详情如下: 

| 配置项       | 类型         | 描述                                                                     |
| ------------ | ------------ | ------------------------------------------------------------------------ |
| CertName     | String       | 服务端证书名称（注：在server_cert_conf.data文件中定义）                  |
| NextProtos   | String Array | 启用的应用层协议(h2, spdy/3.1, http/1.1)，要求满足以下规则：<br>- 配置为空时：默认使用"http/1.1"<br>- 协议列表要使用"spdy/3.1","h2","http/1.1"中的一个或多个作为配置选项，并且至少要包含"http/1.1" |
| Grade        | String       | 安全等级                                                                 |
| ClientAuth   | Bool         | 是否启用双向认证                                                         |
| ClientCAName | String       | 客户端证书签发CA名称                                                     |
| VipConf      | String Array | 产品线VIP列表（注：优先依据VIP来确定TLS配置）                            |
| SniConf      | String Array | 产品线域名列表，可选（注：无法依据VIP确定TLS配置时，使用SNI确定TLS配置） |

# 示例

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
                "h2",
                "http/1.1"
            ],
            "Grade": "B",
            "ClientCAName": ""
        }
    }
}
```
