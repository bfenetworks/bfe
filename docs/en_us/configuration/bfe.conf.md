# Core Configuration

## Introduction

bfe.conf is the core configuration file of BFE.

## Configuration

### Server basic config

| Config Item                   | Description                                                  |
| ----------------------------- | ------------------------------------------------------------ |
| Server.HttpPort               | Integer<br>Listen port for HTTP<br>Default 8080 |
| Server.HttpsPort               | Integer<br>Listen port for HTTPS<br>Default 8443 |
| Server.MonitorPort             | Integer<br>Listen port for monitor<br>Default 8421 |
| Server.MonitorEnabled          | Boolean<br>If false, monitor server is disabled<br>Default True |
| Server.MaxCpus                 | Integer<br>Max number of CPUs to use (0 to use all CPUs)<br>Default 0 |
| Server.Layer4LoadBalancer      | String<br>Type of layer-4 load balancer (PROXY/NONE)<br>Default NONE |
| Server.TlsHandshakeTimeout     | Integer<br>TLS handshake timeout, in seconds<br>Default 30 |
| Server.ClientReadTimeout       | Integer<br>Read timeout of communicating with http client, in seconds<br>Default 60 |
| Server.ClientWriteTimeout      | Integer<br>Write timeout of communicating with http client, in seconds<br>Default 60 |
| Server.KeepAliveEnabled        | Boolean<br>If false, HTTP Keep-Alive is disabled<br>Default True |
| Server.GracefulShutdownTimeout | Integer<br>Timeout for graceful shutdown (maximum 300 sec)<br>Default 10 |
| Server.MaxHeaderBytes          | Integer<br>Max length of request header, in bytes<br>Default 1048576 |
| Server.MaxHeaderUriBytes       | Integer<br>Max length of request URI, in bytes<br>Default 8192 |
| Server.HttpAddr                | String<br>Listen address for HTTP; empty means all addresses<br>Default "" |
| Server.HttpsAddr               | String<br>Listen address for HTTPS; empty means all addresses<br>Default "" |
| Server.MonitorAddr             | String<br>Listen address for monitor; empty means all addresses<br>Default "" |
| Server.AcceptNum               | Integer<br>Number of accept goroutines per listener<br>Default 1 |
| Server.MaxProxyHeaderBytes     | Integer<br>Max length of PROXY protocol header, in bytes<br>Default 0 |
| Server.EnableAiGateway         | Boolean<br>Enable AI Gateway mode<br>Default False |
| Server.EstimateToken           | Boolean<br>Estimate token usage based on request Content-Length<br>Default False |
| Server.HostRuleConf            | String<br>Path of [host config](server_data_conf/host_rule.data.md)<br>Default server_data_conf/host_rule.data |
| Server.VipRuleConf             | String<br>Path of [VIP config](server_data_conf/vip_rule.data.md)<br>Default server_data_conf/vip_rule.data |
| Server.RouteRuleConf           | String<br>Path of [route rule config](server_data_conf/route_rule.data.md)<br>Default server_data_conf/route_rule.data |
| Server.ClusterConf             | String<br>Path of [cluster config](server_data_conf/cluster_conf.data.md)<br>Default server_data_conf/cluster_conf.data |
| Server.GslbConf                | String<br>Path of [subcluster balancing config](cluster_conf/gslb.data.md)<br>Default cluster_conf/gslb.data |
| Server.ClusterTableConf        | String<br>Path of [instance balancing config](cluster_conf/cluster_table.data.md)<br>Default cluster_conf/cluster_table.data |
| Server.NameConf                | String<br>Path of [naming config](server_data_conf/name_conf.data.md)<br>Default server_data_conf/name_conf.data |
| Server.Modules                 | String<br>Enabled modules; to enable multiple modules, add multiple Modules lines, see configuration example<br>Default "" |
| Server.MonitorInterval         | Integer<br>Interval for get diff of proxy-state<br>Default 20 |
| Server.DebugServHttp           | Boolean<br>Debug flag for ServerHttp<br>Default False |
| Server.DebugBfeRoute           | Boolean<br>Debug flag for BfeRoute<br>Default False |
| Server.DebugBal                | Boolean<br>Debug flag for Bal<br>Default False |
| Server.DebugHealthCheck        | Boolean<br>Debug flag for HealthCheck<br>Default False |

### TLS basic config

| Config Item                       | Description                                                      |
| --------------------------------- | ---------------------------------------------------------------- |
| HttpsBasic.ServerCertConf         | String<br>Path of [cert config](tls_conf/server_cert_conf.data.md)<br>Default tls_conf/server_cert_conf.data |
| HttpsBasic.TlsRuleConf            | String<br>Path of [tls rule config](tls_conf/tls_rule_conf.data.md)<br>Default tls_conf/tls_rule_conf.data |
| HttpsBasic.CipherSuites           | String<br>CipherSuites preference settings<br>Default TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256&#124;TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256&#124;TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256&#124;TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |
| HttpsBasic.CurvePreferences       | String<br>Curve preference settings<br>Default CurveP256 |
| HttpsBasic.EnableSslv2ClientHello | Boolean<br>Enable Sslv2ClientHello for compatible with ancient sslv3 client<br>Default True |
| HttpsBasic.ClientCABaseDir        | String<br>Base directory of client ca certificates <br>Note: filename suffix of ca certificate must be ".crt"<br>Default tls_conf/client_ca |
| HttpsBasic.MaxTlsVersion          | String<br>Maximum supported TLS version<br>Default TLSv1.2 |
| HttpsBasic.MinTlsVersion          | String<br>Minimum supported TLS version<br>Default SSLv3 |
| HttpsBasic.ClientCRLBaseDir       | String<br>Base directory of client CRL<br>Default tls_conf/client_crl |
| SessionCache.SessionCacheDisabled | Boolean<br>Disable tls session cache or not<br>Default True |
| SessionCache.Servers              | String<br>Address of cache server<br>No default |
| SessionCache.KeyPrefix            | String<br>Prefix for cache key<br>Default bfe |
| SessionCache.ConnectTimeout       | Integer<br>Connection timeout (ms) <br>Default 50 |
| SessionCache.ReadTimeout          | Integer<br>Read timeout of connection with redis server (ms)<br>No default (value 0 fails validation; must be explicitly configured) |
| SessionCache.WriteTimeout         | Integer<br>Write timeout of connection with redis server (ms)<br>Default 50 |
| SessionCache.MaxIdle              | Integer<br>Max idle connections in connection pool<br>Default 20 |
| SessionCache.SessionExpire        | Integer<br>Expire time for tls session state (second)<br>Default 3600 |
| SessionTicket.SessionTicketsDisabled | Boolean<br>Disable tls session ticket or not<br>Default True |
| SessionTicket.SessionTicketKeyFile   | String<br>Path of [session ticket key config](tls_conf/session_ticket_key.data.md)<br>Default tls_conf/session_ticket_key.data |

## Example

```ini
[Server]
# listen port for http request
HttpPort = 8080
# listen port for https request
HttpsPort = 8443
# listen port for monitor request
MonitorPort = 8421

# max number of CPUs to use (0 to use all CPUs)
MaxCpus = 0

# type of layer-4 load balancer (PROXY/NONE)
# 
# Note:
# - PROXY: layer-4 balancer talking the proxy protocol
#          eg. F5 BigIP/Citrix ADC 
# - NONE: layer-4 balancer disabled 
Layer4LoadBalancer = ""

# tls handshake timeout, in seconds
TlsHandshakeTimeout = 30

# read timeout, in seconds
ClientReadTimeout = 60

# write timeout, in seconds
ClientWriteTimeout = 60

# if false, client connection is shutdown disregard of http headers
KeepAliveEnabled = true

# timeout for graceful shutdown (maximum 300 sec)
GracefulShutdownTimeout = 10

# max header length in bytes in request
MaxHeaderBytes = 1048576

# max URI(in header) length in bytes in request
MaxHeaderUriBytes = 8192

# routing related conf
HostRuleConf = server_data_conf/host_rule.data
VipRuleConf = server_data_conf/vip_rule.data
RouteRuleConf = server_data_conf/route_rule.data
ClusterConf = server_data_conf/cluster_conf.data

# load balancing related conf
GslbConf = cluster_conf/gslb.data
ClusterTableConf = cluster_conf/cluster_table.data

# naming related conf
NameConf = server_data_conf/name_conf.data

Modules = mod_trust_clientip
Modules = mod_block
Modules = mod_header
Modules = mod_rewrite
Modules = mod_redirect
Modules = mod_logid

# interval for get diff of proxy-state
MonitorInterval = 20

DebugServHttp = false
DebugBfeRoute = false
DebugBal = false
DebugHealthCheck = false

[HttpsBasic]
# cert conf for https
ServerCertConf = tls_conf/server_cert_conf.data

# tls rule for https
TlsRuleConf = tls_conf/tls_rule_conf.data

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
#    CipherSuites=TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256|TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
#    CipherSuites=TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256|TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
#
CipherSuites=TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256|TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256
CipherSuites=TLS_ECDHE_RSA_WITH_RC4_128_SHA
CipherSuites=TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA
CipherSuites=TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA
CipherSuites=TLS_RSA_WITH_RC4_128_SHA
CipherSuites=TLS_RSA_WITH_AES_128_CBC_SHA
CipherSuites=TLS_RSA_WITH_AES_256_CBC_SHA

# supported curve preference settings
#
# curves implemented in golang: 
#     CurveP256 
#     CurveP384 
#     CurveP521
#
# Note:
# - Do not use CurveP384/CurveP521 which is with poor performance
#
CurvePreferences=CurveP256

# support Sslv2 ClientHello for compatible with ancient 
# TLS capable clients (mozilla 5, java 5/6, openssl 0.9.8 etc)
EnableSslv2ClientHello = true

# client ca certificates base directory
# Note: filename suffix for ca certificate file should be ".crt", eg. example_ca_bundle.crt
ClientCABaseDir = tls_conf/client_ca

[SessionCache]
# disable tls session cache or not
SessionCacheDisabled = true

# tcp address of redis server
Servers = "example.redis.cluster"

# prefix for cache key
KeyPrefix = "bfe"

# connection params (ms)
ConnectTimeout = 50
ReadTimeout = 50
WriteTimeout = 50

# max idle connections in connection pool
MaxIdle = 20

# expire time for tls session state (second)
SessionExpire = 3600

[SessionTicket]
# disable tls session ticket or not
SessionTicketsDisabled = true
# session ticket key
SessionTicketKeyFile = tls_conf/session_ticket_key.data
```
