# TLS服务端证书配置

## 配置简介

server_cert_conf.data用于配置证书和密钥。

## 配置描述

| 配置项                              | 类型   | 参数含义                 | 必填 | 补充描述                                                     | 合法性条件                                                   |
| ----------------------------------- | ------ | ------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version                             | String | 配置文件版本             | Y    | 参见 [Version](../00-common.md#5-配置文件版本version) 类型定义  | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| Config                              | Object | 证书配置信息             | Y    | 包含 Default 和 CertConf                                     | 非空                                                         |
| Config.Default                      | String | 默认证书名称             | Y    | 默认证书须包含在证书列表 CertConf 中                         | 非空；禁止命名为 `BFE_DEFAULT_CERT`；须在 CertConf 中存在   |
| Config.CertConf                     | Object | 所有证书列表             | Y    | 键为证书名称，值为证书相关文件路径                           | 非空                                                         |
| Config.CertConf{k}                  | String | 证书名称                 | Y    | 作为 CertConf 的键                                           | 非空；禁止命名为 `BFE_DEFAULT_CERT`                          |
| Config.CertConf{v}                  | Object | 证书相关文件路径         | Y    | 包含 ServerCertFile、ServerKeyFile、OcspResponseFile         | 非空                                                         |
| Config.CertConf{v}.ServerCertFile   | String | 证书文件路径             | Y    | 参见 [FilePath](../00-common.md#3-文件路径filepath) 类型定义    | 类型为 [FilePath](../00-common.md#3-文件路径filepath)           |
| Config.CertConf{v}.ServerKeyFile    | String | 证书关联密钥文件路径     | Y    | 参见 [FilePath](../00-common.md#3-文件路径filepath) 类型定义    | 类型为 [FilePath](../00-common.md#3-文件路径filepath)           |
| Config.CertConf{v}.OcspResponseFile | String | 证书关联OCSP Staple文件路径 | N | 可选配置；参见 [FilePath](../00-common.md#3-文件路径filepath) 类型定义 | 类型为 [FilePath](../00-common.md#3-文件路径filepath)           |

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
