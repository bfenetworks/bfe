# Introduction

bfe.conf is the core config file of BFE.

# Configuration

## Server Config

| Config Item             | Type   | Description                                                  |
| ----------------------- | ------ | ------------------------------------------------------------ |
| HttpPort                | Int    | Listen port for HTTP                                         |
| HttpsPort               | Int    | Listen port for HTTPS                                        |
| MonitorPort             | Int    | Listen port for monitor                                      |
| MaxCpus                 | Int    | Max number of CPUs to use (0 to use all CPUs)                |
| Layer4LoadBalancer      | String | Type of layer-4 load balancer (PROXY/BGW/NONE)               |
| TlsHandshakeTimeout     | Int    | TLS handshake timeout, in seconds                            |
| ClientReadTimeout       | Int    | Read timeout of communicating with http client, in seconds   |
| ClientWriteTimeout      | Int    | Write timeout of communicating with http client, in seconds  |
| KeepAliveEnabled        | Bool   | If false, HTTP Keep-Alive is disabled                        |
| GracefulShutdownTimeout | Int    | Timeout for graceful shutdown (maximum 300 sec)              |
| MaxHeaderBytes          | Int    | Max length of request header, in bytes                       |
| MaxHeaderUriBytes       | Int    | Max lenght of request URI, in bytes                          |
| HostRuleConf            | String | Path of host config                                          |
| VipRuleConf             | String | Path of VIP config                                           |
| RouteRuleConf           | String | Path of route rule config                                    |
| ClusterConf             | String | Path of cluster config                                       |
| ClusterTableConf        | String | Path of cluster table config                                 |
| GslbConf                | String | Path of gslb config                                          |
| NameConf                | String | Path of naming config                                        |
| Modules                 | String | Enabled modules                                              |
| MonitorInterval         | Int    | Interval for get diff of proxy-state                         |
| DebugServHttp           | Bool   | Debug flag for ServerHttp                                    |
| DebugBfeRoute           | Bool   | Debug flag for BfeRoute                                      |
| DebugBal                | Bool   | Debug flag for Bal                                           |
| DebugHealthCheck        | Bool   | Debug flag for HealthCheck                                   |

## HttpsBasic Config

| Config Item            | Type   | Description                                                      |
| ---------------------- | ------ | ---------------------------------------------------------------- |
| ServerCertConf         | String | Path of cert config                                              |
| TlsRuleConf            | String | Path of tls rule config                                          |
| CipherSuites           | String | CipherSuites preference settings                                 |
| CurvePreferences       | String | Curve perference settings                                        |
| EnableSslv2ClientHello | Bool   | Enable Sslv2ClientHello for compatible with ancient sslv3 client |
| ClientCABaseDir        | String | Base directory of client ca certificates <br>Note: filename suffix of ca certificate must be ".crt" |

## SessionCache Config

| Config Item            | Type   | Description                                                 |
| ---------------------- | ------ | ----------------------------------------------------------- |
| SessionCacheDisabled   | Bool   | Disable tls session cache or not                            |
| Servers                | String | Address of cache server                                     |
| KeyPrefix              | String | Prefix for cache key                                        |
| ConnectTimeout         | Int    | Connection timeout                                          |
| ReadTimeout            | Int    | Read timeout of connection with redis server                |
| WriteTimeout           | Int    | Write timeout of connection with redis server               |
| MaxIdle                | Int    | Max idle connections in connection pool                     |
| SessionExpire          | Int    | Expire time for tls session state (second)                  |

## SessionTicket Config

| Config Item            | Type   | Description                                                 |
| ---------------------- | ------ | ----------------------------------------------------------- |
| SessionTicketsDisabled | Bool   | Disable tls session ticket or not                           |
| SessionTicketKeyFile   | String | File path of session ticket key                             |


# Example

```
[server]
# listen port for http request
httpPort = 8080
# listen port for https request
httpsPort = 8443
# listen port for monitor request
monitorPort = 8421

# max number of CPUs to use (0 to use all CPUs)
maxCpus = 0

# type of layer-4 load balancer (PROXY/BGW/NONE)
# 
# Note:
# - PROXY: layer-4 balancer talking the proxy protocol
#          eg. F5 BigIP/Citrix ADC 
# - BGW: Baidu GateWay
# - NONE: layer-4 balancer disabled 
layer4LoadBalancer = ""

# tls handshake timeout, in seconds
tlsHandshakeTimeout = 30

# read timeout, in seconds
clientReadTimeout = 60

# write timeout, in seconds
clientWriteTimeout = 60

# if false, client connection is shutdown disregard of http headers
keepAliveEnabled = true

# timeout for graceful shutdown (maximum 300 sec)
gracefulShutdownTimeout = 10

# max header length in bytes in request
maxHeaderBytes = 1048576

# max URI(in header) length in bytes in request
maxHeaderUriBytes = 8192

# routing related confs
hostRuleConf = server_data_conf/host_rule.data
vipRuleConf = server_data_conf/vip_rule.data
routeRuleConf = server_data_conf/route_rule.data
clusterConf = server_data_conf/cluster_conf.data
nameConf = server_data_conf/name_conf.data

# load balancing related confs 
clusterTableConf = cluster_conf/cluster_table.data
gslbConf = cluster_conf/gslb.data

modules = mod_trust_clientip
modules = mod_block
modules = mod_header
modules = mod_rewrite
modules = mod_redirect
modules = mod_logid

# interval for get diff of proxy-state
monitorInterval = 20

debugServHttp = false
debugBfeRoute = false
debugBal = false
debugHealthCheck = false

[httpsBasic]
# cert conf for https
serverCertConf = tls_conf/server_cert_conf.data

# tls rule for https
tlsRuleConf = tls_conf/tls_rule_conf.data

# supported cipherSuites preference settings
#
# ciphersuites implemented in golang
#     TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
#     TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
#     TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
#     TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
#     TLS_ECDHE_RSA_WITH_RC4_128_SHA
#     TLS_ECDHE_ECDSA_WITH_RC4_128_SHA
#     TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA
#     TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA
#     TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA
#     TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA
#     TLS_RSA_WITH_RC4_128_SHA
#     TLS_RSA_WITH_AES_128_CBC_SHA
#     TLS_RSA_WITH_AES_256_CBC_SHA
#     TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA
#     TLS_RSA_WITH_3DES_EDE_CBC_SHA
#
# Note:
# -. Equivalent cipher suites (cipher suites with same priority in server side):
#    cipherSuites=TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256|TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
#    cipherSuites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256|TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
#
cipherSuites=TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256|TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
cipherSuites=TLS_ECDHE_RSA_WITH_RC4_128_SHA
cipherSuites=TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA
cipherSuites=TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA
cipherSuites=TLS_RSA_WITH_RC4_128_SHA
cipherSuites=TLS_RSA_WITH_AES_128_CBC_SHA
cipherSuites=TLS_RSA_WITH_AES_256_CBC_SHA

# supported curve perference settings
#
# curves implemented in golang: 
#     CurveP256 
#     CurveP384 
#     CurveP521
#
# Note:
# - Do not use CurveP384/CurveP521 which is with poor performance
#
curvePreferences=CurveP256

# support Sslv2 ClientHello for compatible with ancient 
# TLS capable clients (mozilla 5, java 5/6, openssl 0.9.8 etc)
enableSslv2ClientHello = true

# client ca certificates base directory
# Note: filename suffix for ca certificate file should be ".crt", eg. example_ca_bundle.crt
clientCABaseDir = tls_conf/client_ca

[sessionCache]
# disable tls session cache or not
sessionCacheDisabled = true

# tcp address of redis server
servers = "example.redis.cluster"

# prefix for cache key
keyPrefix = "bfe"

# connection params (ms)
connectTimeout = 50
readTimeout = 50
writeTimeout = 50

# max idle connections in connection pool
maxIdle = 20

# expire time for tls session state (second)
sessionExpire = 3600

[sessionTicket]
# disable tls session ticket or not
sessionTicketsDisabled = true
# session ticket key
sessionTicketKeyFile = tls_conf/session_ticket_key.data
```
