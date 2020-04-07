# Introduction

bfe.conf is the core configuration file of BFE.

# Configuration

## Server basic config

| Config Item                   | Description                                                  |
| ----------------------------- | ------------------------------------------------------------ |
| Basic.HttpPort                | Integer<br>Listen port for HTTP<br>Default 8080 |
| Basic.HttpsPort               | Integer<br>Listen port for HTTPS<br>Default 8443 |
| Basic.MonitorPort             | Integer<br>Listen port for monitor<br>Default 8421 |
| Basic.MaxCpus                 | Integer<br>Max number of CPUs to use (0 to use all CPUs)<br>Default 0 |
| Basic.Layer4LoadBalancer      | String<br>Type of layer-4 load balancer (PROXY/BGW/NONE)<br>Default NONE |
| Basic.TlsHandshakeTimeout     | Integer<br>TLS handshake timeout, in seconds<br>Default 30 |
| Basic.ClientReadTimeout       | Integer<br>Read timeout of communicating with http client, in seconds<br>Default 60 |
| Basic.ClientWriteTimeout      | Integer<br>Write timeout of communicating with http client, in seconds<br>Default 60 |
| Basic.KeepAliveEnabled        | Boolean<br>If false, HTTP Keep-Alive is disabled<br>Default True |
| Basic.GracefulShutdownTimeout | Integer<br>Timeout for graceful shutdown (maximum 300 sec)<br>Default 10 |
| Basic.MaxHeaderBytes          | Integer<br>Max length of request header, in bytes<br>Default 10485 |
| Basic.MaxHeaderUriBytes       | Integer<br>Max lenght of request URI, in bytes<br>Default 8192 |
| Basic.HostRuleConf            | String<br>Path of host config<br>Default server_data_conf/host_rule.data |
| Basic.VipRuleConf             | String<br>Path of VIP config<br>Default server_data_conf/vip_rule.data |
| Basic.RouteRuleConf           | String<br>Path of route rule config<br>Default server_data_conf/route_rule.data |
| Basic.ClusterConf             | String<br>Path of cluster config<br>Default server_data_conf/cluster_conf.data |
| Basic.ClusterTableConf        | String<br>Path of cluster table config<br>Default cluster_conf/cluster_table.data |
| Basic.GslbConf                | String<br>Path of gslb config<br>Default cluster_conf/gslb.data |
| Basic.NameConf                | String<br>Path of naming config<br>Default server_data_conf/name_conf.data |
| Basic.Modules                 | String<br>Enabled modules<br>Default "" |
| Basic.MonitorInterval         | Integer<br>Interval for get diff of proxy-state<br>Default 20 |
| Basic.DebugServHttp           | Boolean<br>Debug flag for ServerHttp<br>Default False |
| Basic.DebugBfeRoute           | Boolean<br>Debug flag for BfeRoute<br>Default False |
| Basic.DebugBal                | Boolean<br>Debug flag for Bal<br>Default False |
| Basic.DebugHealthCheck        | Boolean<br>Debug flag for HealthCheck<br>Default False |

## TLS basic config

| Config Item                       | Description                                                      |
| --------------------------------- | ---------------------------------------------------------------- |
| HttpsBasic.ServerCertConf         | String<br>Path of cert config<br>Default tls_conf/server_cert_conf.data |
| HttpsBasic.TlsRuleConf            | String<br>Path of tls rule config<br>Default tls_conf/tls_rule_conf.data |
| HttpsBasic.CipherSuites           | String<br>CipherSuites preference settings<br>Default                                   |
| HttpsBasic.CurvePreferences       | String<br>Curve perference settings<br>Default CurveP256 |
| HttpsBasic.EnableSslv2ClientHello | Boolean<br>Enable Sslv2ClientHello for compatible with ancient sslv3 client<br>Default True |
| HttpsBasic.ClientCABaseDir        | String<br>Base directory of client ca certificates <br>Note: filename suffix of ca certificate must be ".crt"<br>Default tls_conf/client_ca |
| SessioCache.SessionCacheDisabled   | Boolean<br>Disable tls session cache or not<br>Default True |
| SessioCache.Servers                | String<br>Address of cache server<br>Default "" |
| SessioCache.KeyPrefix              | String<br>Prefix for cache key<br>Default bfe |
| SessioCache.ConnectTimeout         | Integer<br>Connection timeout (ms) <br>Default 50 |
| SessioCache.ReadTimeout            | Integer<br>Read timeout of connection with redis server (ms)<br>Default 50 |
| SessioCache.WriteTimeout           | Integer<br>Write timeout of connection with redis server (ms)<br>Default 50 |
| SessioCache.MaxIdle                | Integer<br>Max idle connections in connection pool<br>Default 20 |
| SessioCache.SessionExpire          | Integer<br>Expire time for tls session state (second)<br>Default 3600 |
| SessionTicket.SessionTicketsDisabled | Boolean<br>Disable tls session ticket or not<br>Default True |
| SessionTicket.SessionTicketKeyFile   | String<br>File path of session ticket key<br>Default tls_conf/session_ticket_key.data |


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
