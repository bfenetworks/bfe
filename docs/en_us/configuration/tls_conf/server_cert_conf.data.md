# Configuration about Server Certificates

## Introduction

server_cert_conf.data records the config for server certificate and private key

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | See [Version](../00-common.md#5-version) type definition | Type must be [Version](../00-common.md#5-version) |
| Config | Object | Server certificate configuration information | Y | Contains Default and CertConf | Non-empty |
| Config.Default | String | Name of default cert | Y | Default cert must be configured<br>Default cert must be included in cert list {CertConf} | Non-empty; cannot be named `BFE_DEFAULT_CERT`; must exist in CertConf |
| Config.CertConf | Object | Cert list | Y | Key is cert name, value is cert related file path | Non-empty |
| Config.CertConf{k} | String | Name of cert | Y | Key of CertConf | Non-empty; cannot be named `BFE_DEFAULT_CERT` |
| Config.CertConf{v} | Object | Cert related file path | Y | Contains ServerCertFile, ServerKeyFile, OcspResponseFile | Non-empty |
| Config.CertConf{v}.ServerCertFile | String | Path of server certificate | Y | See [FilePath](../00-common.md#3-filepath) type definition | Type must be [FilePath](../00-common.md#3-filepath) |
| Config.CertConf{v}.ServerKeyFile | String | Path of private key | Y | See [FilePath](../00-common.md#3-filepath) type definition | Type must be [FilePath](../00-common.md#3-filepath) |
| Config.CertConf{v}.OcspResponseFile | String | Path of OCSP Staple (optional) | N | Optional; see [FilePath](../00-common.md#3-filepath) type definition | Type must be [FilePath](../00-common.md#3-filepath) |

## Example

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
