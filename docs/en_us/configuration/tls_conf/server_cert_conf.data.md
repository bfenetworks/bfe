# Configuration about Server Certificates

## Introduction

server_cert_conf.data records the config for server certificate and private key

## Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of configure file                          |
| Config      | Object<br>Server certificate configuration information       |
| Config.Default  | String<br>Name of default cert <br>- Default cert must be configured <br>- Default cert must be included in cert list {CertConf}  |
| Config.CertConf | Object<br>Cert list  |
| Config.CertConf{k}    | String<br>Name of cert <br>- Cert name can not be "BFE_DEFAULT_CERT"  |
| Config.CertConf{v}    | Object<br>Cert related file path                                      |
| Config.CertConf{v}.ServerCertFile    | String<br>Path of server certificate    |
| Config.CertConf{v}.ServerKeyFile     | String<br>Path of private key           |
| Config.CertConf{v}.OcspResponseFile  | String<br>Path of OCSP Stple (optional) |

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
