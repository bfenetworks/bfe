# Core Configuration

## Introduction

bfe.conf is the core configuration file of BFE.

## Configuration

### Server basic config

| Configuration Item             | Type    | Meaning                                              | Required  | Supplementary Description                                                                                      | Validity Condition                                                   |
| ------------------------------ | ------- | ---------------------------------------------------- | --------- | -------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Server.HttpPort                | Integer | Listen port for HTTP                                 | N         | Default 8080; see [Port](00-common.md#1-port) type definition                                                  | Type is [Port](00-common.md#1-port), value range [1, 65535]          |
| Server.HttpsPort               | Integer | Listen port for HTTPS                                | N         | Default 8443; see [Port](00-common.md#1-port) type definition                                                  | Type is [Port](00-common.md#1-port), value range [1, 65535]          |
| Server.MonitorPort             | Integer | Listen port for monitor                              | N         | Default 8421; see [Port](00-common.md#1-port) type definition                                                  | Type is [Port](00-common.md#1-port); value range [1, 65535] when `MonitorEnabled=true` |
| Server.MonitorEnabled          | Boolean | Whether monitor server is enabled                    | N         | Default `True`                                                                                                 | -                                                                    |
| Server.MaxCpus                 | Integer | Max number of CPUs to use                            | N         | Default 0; 0 means use all CPU cores                                                                         | >= 0                                                                 |
| Server.Layer4LoadBalancer      | String  | Type of layer-4 load balancer                        | N         | Default `NONE`                                                                                                 | Only `PROXY` / `NONE` supported                                      |
| Server.TlsHandshakeTimeout     | Integer | TLS handshake timeout, in seconds                    | N         | Default 30                                                                                                     | > 0 and <= 1200                                                      |
| Server.ClientReadTimeout       | Integer | Read timeout of communicating with HTTP client, in seconds | N   | Default 60                                                                                                     | > 0                                                                  |
| Server.ClientWriteTimeout      | Integer | Write timeout of communicating with HTTP client, in seconds | N  | Default 60                                                                                                     | > 0                                                                  |
| Server.KeepAliveEnabled        | Boolean | Whether HTTP Keep-Alive is enabled for client connection | N     | Default `True`                                                                                                 | -                                                                    |
| Server.GracefulShutdownTimeout | Integer | Timeout for graceful shutdown, in seconds            | N         | Default 10                                                                                                     | (0, 300]                                                             |
| Server.MaxHeaderBytes          | Integer | Max length of request header, in bytes               | N         | Default 1048576                                                                                                | > 0                                                                  |
| Server.MaxHeaderUriBytes       | Integer | Max length of request URI in header, in bytes        | N         | Default 8192                                                                                                   | > 0                                                                  |
| Server.HttpAddr                | String  | Listen address for HTTP                              | N         | See [ListenAddr](00-common.md#2-listenaddr) type definition                                                    | Type is [ListenAddr](00-common.md#2-listenaddr)                      |
| Server.HttpsAddr               | String  | Listen address for HTTPS                             | N         | See [ListenAddr](00-common.md#2-listenaddr) type definition                                                    | Type is [ListenAddr](00-common.md#2-listenaddr)                      |
| Server.MonitorAddr             | String  | Listen address for monitor                           | N         | See [ListenAddr](00-common.md#2-listenaddr) type definition                                                    | Type is [ListenAddr](00-common.md#2-listenaddr)                      |
| Server.AcceptNum               | Integer | Number of accept goroutines per listener             | N         | Default 1; automatically set to 1 when 0                                                                       | >= 0                                                                 |
| Server.MaxProxyHeaderBytes     | Integer | Max length of PROXY protocol header, in bytes        | N         | Default 0                                                                                                      | >= 0                                                                 |
| Server.EnableAiGateway         | Boolean | Whether AI Gateway mode is enabled                   | N         | Default `False`                                                                                                | -                                                                    |
| Server.EstimateToken           | Boolean | Whether to estimate token usage based on request Content-Length | N | Default `False`                                                                                                | -                                                                    |
| Server.AccessibleBodySize      | Integer | Max size of request body that can be buffered, in bytes | N      | Default 2097152; used for request body rewriting and AI Gateway fallback retry; request bodies larger than this cannot be fully cached and retransmitted | > 0 and <= 8388608                                                   |
| Server.TotalBodyBufferSize     | Integer | Upper limit of total memory used by all active bytes_body buffers, in bytes | N      | Default 0 (unlimited); when reached, AI Gateway fallback will not wrap the request body for caching, i.e., no retry | >= 0                                                                 |
| Server.HostRuleConf            | String  | Path of [host config](server_data_conf/host_rule.data.md) file | N     | Default `server_data_conf/host_rule.data`; see [FilePath](00-common.md#3-filepath) type definition            | Type is [FilePath](00-common.md#3-filepath)                          |
| Server.VipRuleConf             | String  | Path of [VIP config](server_data_conf/vip_rule.data.md) file | N       | Default `server_data_conf/vip_rule.data`; see [FilePath](00-common.md#3-filepath) type definition             | Type is [FilePath](00-common.md#3-filepath)                          |
| Server.RouteRuleConf           | String  | Path of [route rule config](server_data_conf/route_rule.data.md) file | N  | Default `server_data_conf/route_rule.data`; see [FilePath](00-common.md#3-filepath) type definition           | Type is [FilePath](00-common.md#3-filepath)                          |
| Server.ClusterConf             | String  | Path of [cluster config](server_data_conf/cluster_conf.data.md) file | N    | Default `server_data_conf/cluster_conf.data`; see [FilePath](00-common.md#3-filepath) type definition         | Type is [FilePath](00-common.md#3-filepath)                          |
| Server.GslbConf                | String  | Path of [subcluster balancing config](cluster_conf/gslb.data.md) file (GSLB) | N | Default `cluster_conf/gslb.data`; see [FilePath](00-common.md#3-filepath) type definition                  | Type is [FilePath](00-common.md#3-filepath)                          |
| Server.ClusterTableConf        | String  | Path of [instance balancing config](cluster_conf/cluster_table.data.md) file | N | Default `cluster_conf/cluster_table.data`; see [FilePath](00-common.md#3-filepath) type definition          | Type is [FilePath](00-common.md#3-filepath)                          |
| Server.NameConf                | String  | Path of [naming config](server_data_conf/name_conf.data.md) file | N     | Optional; not loaded if not configured; see [FilePath](00-common.md#3-filepath) type definition               | Type is [FilePath](00-common.md#3-filepath)                          |
| Server.Modules                 | String  | List of enabled modules                              | N         | Default empty; to enable multiple modules, add multiple Modules lines, see configuration example               | -                                                                    |
| Server.MonitorInterval         | Integer | Monitor data statistics interval, in seconds         | N         | Default 20; must divide 60; values greater than 60 will be truncated to 60                                     | [20, 60] and must divide 60                                          |
| Server.DebugServHttp           | Boolean | Whether to enable debug log for reverse proxy module | N         | Default `False`                                                                                                | -                                                                    |
| Server.DebugBfeRoute           | Boolean | Whether to enable debug log for traffic routing module | N       | Default `False`                                                                                                | -                                                                    |
| Server.DebugBal                | Boolean | Whether to enable debug log for load balancing module | N        | Default `False`                                                                                                | -                                                                    |
| Server.DebugHealthCheck        | Boolean | Whether to enable debug log for health check module  | N         | Default `False`                                                                                                | -                                                                    |

### TLS basic config

| Configuration Item                   | Type      | Meaning                                                                         | Required   | Supplementary Description                                                                                      | Validity Condition                                                                  |
| ------------------------------------ | --------- | ------------------------------------------------------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| HttpsBasic.ServerCertConf            | String    | Path of [server cert and key config](tls_conf/server_cert_conf.data.md) file    | N          | Default `tls_conf/server_cert_conf.data`; see [FilePath](00-common.md#3-filepath) type definition             | Type is [FilePath](00-common.md#3-filepath)                                         |
| HttpsBasic.TlsRuleConf               | String    | Path of [TLS rule config](tls_conf/tls_rule_conf.data.md) file                  | N          | Default `tls_conf/tls_rule_conf.data`; see [FilePath](00-common.md#3-filepath) type definition                | Type is [FilePath](00-common.md#3-filepath)                                         |
| HttpsBasic.CipherSuites              | String[]  | List of enabled cipher suites                                                   | N          | To enable multiple suites, add multiple `CipherSuites` lines; equivalent suites can be separated by `&#124;`, see example | Must be cipher suites supported by BFE, e.g. `TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256` etc. |
| HttpsBasic.CurvePreferences          | String[]  | List of enabled ECC curves                                                      | N          | Default `CurveP256`                                                                                            | Only `CurveP256`, `CurveP384`, `CurveP521` supported                                |
| HttpsBasic.EnableSslv2ClientHello    | Boolean   | Enable SSLv2 format ClientHello compatibility for SSLv3 protocol                | N          | Default `True`                                                                                                 | -                                                                                   |
| HttpsBasic.ClientCABaseDir           | String    | Base directory of client CA certificates                                        | N          | Default `tls_conf/client_ca`; certificate files in the directory must have `.crt` suffix; see [DirPath](00-common.md#4-dirpath) type definition | Type is [DirPath](00-common.md#4-dirpath)                                         |
| HttpsBasic.MaxTlsVersion             | String    | Highest supported TLS version                                                   | N          | Default `VersionTLS12`                                                                                         | Only `VersionSSL30`, `VersionTLS10`, `VersionTLS11`, `VersionTLS12` supported       |
| HttpsBasic.MinTlsVersion             | String    | Lowest supported TLS version                                                    | N          | Default `VersionSSL30`                                                                                         | Only the above enum values; and `MaxTlsVersion >= MinTlsVersion`                    |
| HttpsBasic.ClientCRLBaseDir          | String    | Base directory of client CRL                                                    | N          | Default `tls_conf/client_crl`; see [DirPath](00-common.md#4-dirpath) type definition                          | Type is [DirPath](00-common.md#4-dirpath)                                           |
| SessionCache.SessionCacheDisabled    | Boolean   | Whether to disable TLS session cache mechanism                                  | N          | Default `True`; when `True`, other SessionCache related validations are skipped                                | -                                                                                   |
| SessionCache.Servers                 | String    | Access address of cache service                                                 | Conditional | Required when `SessionCacheDisabled=false`                                                                     | Cannot be empty when `SessionCacheDisabled=false`; multiple addresses separated by comma `,` |
| SessionCache.KeyPrefix               | String    | Prefix for cache key                                                            | N          | Default `bfe`                                                                                                  | -                                                                                   |
| SessionCache.ConnectTimeout          | Integer   | Connection timeout to cache service, in milliseconds                            | N          | Default 50                                                                                                     | -                                                                                   |
| SessionCache.ReadTimeout             | Integer   | Read timeout from cache service, in milliseconds                                | Conditional | Required when `SessionCacheDisabled=false`                                                                     | Must be > 0 when `SessionCacheDisabled=false`                                       |
| SessionCache.WriteTimeout            | Integer   | Write timeout to cache service, in milliseconds                                 | Conditional | Required when `SessionCacheDisabled=false`                                                                     | Must be > 0 when `SessionCacheDisabled=false`                                       |
| SessionCache.MaxIdle                 | Integer   | Max idle long connections to cache service                                      | Conditional | Required when `SessionCacheDisabled=false`                                                                     | Must be > 0 when `SessionCacheDisabled=false`                                       |
| SessionCache.SessionExpire           | Integer   | Expiration time of session info stored in cache service, in seconds             | Conditional | Required when `SessionCacheDisabled=false`                                                                     | Must be > 0 when `SessionCacheDisabled=false`                                       |
| SessionTicket.SessionTicketsDisabled | Boolean   | Whether to disable TLS session ticket                                           | N          | Default `True`; when `True`, other SessionTicket related validations are skipped                               | -                                                                                   |
| SessionTicket.SessionTicketKeyFile   | String    | Path of [session ticket key config](tls_conf/session_ticket_key.data.md) file   | N          | Default `tls_conf/session_ticket_key.data`; see [FilePath](00-common.md#3-filepath) type definition           | Type is [FilePath](00-common.md#3-filepath)                                         |

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

# max request body size that can be buffered for rewriting/fallback (default 2MB, max 8MB)
AccessibleBodySize = 2097152

# max total bytes of all active bytes_body buffers (0 means unlimited)
TotalBodyBufferSize = 0

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
