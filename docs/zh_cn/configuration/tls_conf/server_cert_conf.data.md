# TLS服务端证书配置

## 配置简介

server_cert_conf.data用于配置证书和密钥。

## 配置描述

| 配置项   | 描述                                                         |
| -------- | ------------------------------------------------------------ |
| Version  | String<br>配置文件版本                                       |
| Config   | Object<br>证书配置信息                                       |
| Config.Default  | String<br>默认证书名称; 必配选项, 默认证书须包含在证书列表(CertConf)中 |
| Config.CertConf | Object<br>所有证书列表 |
| Config.CertConf{k} | String<br>证书名称; 证书名称禁止命名为"BFE_DEFAULT_CERT" |
| Config.CertConf{v} | Object<br>证书相关文件路径 |
| Config.CertConf{v}.ServerCertFile | String<br>证书文件路径 |
| Config.CertConf{v}.ServerKeyFile | String<br>证书关联密钥文件路径 |
| Config.CertConf{v}.OcspResponseFile | String<br>证书关联OCSP Stple文件路径<br>可选配置 |

## 配置示例

```json
{
    "Version": "20190101000000",
    "Config": {
        "Default": "example.org",
        "CertConf": {
            "example.org": {
                "ServerCertFile": "tls_conf/certs/server.crt",
                "ServerKeyFile" : "tls_conf/certs/server.key"
            }
        }
    }
}
```
