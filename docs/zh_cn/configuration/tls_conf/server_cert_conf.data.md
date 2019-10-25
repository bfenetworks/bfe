# 简介

server_cert_conf.data用于配置证书和密钥。

# 配置

| 配置项   | 类型   | 描述                                                         |
| -------- | ------ | ------------------------------------------------------------ |
| Version  | String | 配置文件版本                                                 |
| Default  | String | 默认证书名称；<br>- 必须要配置默认证书<br>- 默认证书必须包含在证书列表(CertConf)中 |
| CertConf | Map&lt;String, Certificate&gt; | 所有证书配置，Key代表证书名称，Value代表证书配置 <br>- 证书名称禁止命名为"BFE_DEFAULT_CERT" |

## Certficate
| 配置项   | 类型   | 描述                                                         |
| -------- | ------ | ------------------------------------------------------------ |
| ServiceCertFile  | String | 证书文件路径                                                 |
| ServiceKeyFile  | String | 私钥文件路径 |
| OcspResponseFile | String | 证书OCSP Stple文件路径（可选） |

# 示例

```
{
    "Version": "20190101000000",
    "Config": {
        "Default": "example.org",
        "CertConf": {
            "example.org": {
                "ServerCertFile": "../conf/tls_conf/certs/server.crt",
                "ServerKeyFile" : "../conf/tls_conf/certs/server.key"
            }
        }
    }
}
```
