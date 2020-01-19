# 简介

tls_rule_conf.data配置TLS协议参数。

# 配置

| 配置项            | 类型         | 描述                                                          |
| ----------------- | ------------ | ------------------------------------------------------------- |
| Version           | String       | 配置文件版本                                                  |
| Config            | Map&lt;String, TLSConf&gt; | TLS协议配置，Key是标签，Value是TLS配置详情             |
| DefaultNextProtos | Array&lt;String&gt; | 默认TLS应用层协议(h2, spdy/3.1, http/1.1)                   |

## TLSConf 

| 配置项       | 类型         | 描述                                                                     |
| ------------ | ------------ | ------------------------------------------------------------------------ |
| CertName     | String       | 服务端证书名称（注：在server_cert_conf.data文件中定义）                  |
| NextProtos   | Array&lt;String&gt; | TLS应用层协议(h2, spdy/3.1, http/1.1); 缺省为["http/1.1"]             |
| Grade        | String       | TLS安全等级（A+，A，B，C）                                               |
| ClientAuth   | Bool         | 是否启用TLS双向认证                                                      |
| ClientCAName | String       | 客户端证书签发CA名称                                                     |
| VipConf      | Array&lt;String&gt; | VIP列表（注：优先依据VIP来确定TLS配置）                                  |
| SniConf      | Array&lt;String&gt; | 域名列表，可选（注：无法依据VIP确定TLS配置时，使用SNI确定TLS配置）       |

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
