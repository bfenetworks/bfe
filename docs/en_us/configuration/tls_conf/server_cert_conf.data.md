# Introduction

server_cert_conf.data records the config for server certificate and private key

# Configuration

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Time of generating config file                               |
| Default     | String<br>Name of default cert. <br>- default cert must be configed    |
| CertConf    | Struct<br>Cert list<br>- cert name can not be "BFE_DEFAULT_CERT"<br>- ServerCertFile: path of server certificate<br>- ServerKeyFile: path of private key<br>- OcspResponseFile: path of OCSP Stple (oprional) |

# Example
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
