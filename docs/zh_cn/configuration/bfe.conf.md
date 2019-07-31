# 简介

bfe.conf是BFE的核心配置。

# 配置

## 服务基础配置

| 配置项                  | 类型   | 描述                                                         |
| ----------------------- | ------ | ------------------------------------------------------------ |
| HttpPort                | Int    | HTTP流量监听端口                                             |
| HttpsPort               | Int    | HTTPS流量监听端口                                            |
| MonitorPort             | Int    | 监控流量监听端口                                             |
| MaxCpus                 | Int    | 最大使用CPU核数; 0代表使用所有CPU核                          |
| Layer4LoadBalancer      | String | 四层负载均衡器类型                                           |
| TlsHandshakeTimeout     | Int    | TLS握手超时时间，单位为秒                                    |
| ClientReadTimeout       | Int    | 读客户端超时时间，单位为秒                                   |
| ClientWriteTimeout      | Int    | 写客户端超时时间，单位为秒                                   |
| GracefulShutdownTimeout | Int    | 优雅退出超时时间，单位为秒，最大300秒                        |
| KeepAliveEnabled        | Bool   | 与用户端连接是否启用HTTP KeepAlive                           |
| MaxHeaderBytes          | Int    | 请求头部的最大程度，单位为Bytes                              |
| MaxHeaderUriBytes       | Int    | 请求头部URI的最大长度，单位为Byte                            |
| HostRuleConf            | String | 租户域名表配置文件                                           |
| VipRuleConf             | String | 租户VIP表配置文件                                            |
| RouteRuleConf           | String | 转发规则配置文件                                             |
| ClusterConf             | String | 后端集群相关配置文件                                         |
| GslbConf                | String | 集群级别负载均衡配置(GSLB)                                   |
| ClusterTableConf        | String | 子集群级别负载均衡配置文件                                   |
| NameConf                | String | 名字与实例映射表配置文件                                     |
| Modules                 | String | 启用的模块列表; 多个模块增加多个Modules即可                  |
| MonitorInterval         | Int    | monitor统计周期                                              |
| DebugServHttp           | Bool   | 是否开启ServHttp调试日志                                     |
| DebugBfeRoute           | Bool   | 是否开启BfeRoute调试日志                                     |
| DebugBal                | Bool   | 是否开启Bal调试日志                                          |
| DebugHealthCheck        | Bool   | 是否开启HealthCheck调试日志                                  |

## HTTPS基础配置

| 配置项                 | 类型   | 描述                                                         |
| ---------------------- | ------ | ------------------------------------------------------------ |
| ServerCertConf         | String | 证书与密钥的配置文件                                         |
| TlsRuleConf            | String | TLS协议参数配置文件                                          |
| CipherSuites           | String | 启用的加密套件列表。多个套件增加多个cipherSuites即可         |
| CurvePreferences       | String | 启用的ECC椭圆曲线。                                          |
| EnableSslv2ClientHello | Bool   | 针对SSLv3协议，启用对SSLv2格式ClientHello的兼容              |
| ClientCABaseDir        | String | 客户端根CA证书目录。<br>注意：证书文件后缀约定必须是 ".crt"  |

## TLS Session Cache相关配置

| 配置项               | 类型   | 描述                                                         |
| -------------------- | ------ | ------------------------------------------------------------ |
| SessionCacheDisabled | Bool   | 是否禁用TLS Session Cache机制                                |
| Servers              | String | Cache Server的访问地址                                       |
| KeyPrefix            | String | 缓存key前缀                                                  |
| ConnectTimeout       | Int    | 连接Cache Server的超时时间, 单位毫秒                         |
| ReadTimeout          | Int    | 读取Cache Server的超时时间, 单位毫秒                         |
| WriteTimeout         | Int    | 写入Cache Server的超时时间, 单位毫秒                         |
| MaxIdle              | Int    | 与Cache Server的最大空闲长连接数                             |
| SessionExpire        | Int    | Cache Server中存储值的过期时间, 单位秒                       |

## TLS Session Ticket相关配置

| 配置项                 | 类型   | 描述                             |
| ---------------------- | ------ | -------------------------------- |
| SessionTicketsDisabled | Bool   | 是否禁用TLS Session Ticket       |
| SessionTicketKeyFile   | String | Session Ticket Key文件路径       |

# 示例

```
[server]
# listen port for http request
httpPort = 8080
# listen port for https request
httpsPort = 8443
# listen port for monitor request
monitorPort = 8299

# max number of CPUs to use (0 to use all CPUs)
maxCpus = 0

# type of layer-4 load balancer
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

# routing related conf
hostRuleConf = server_data_conf/host_rule.data
vipRuleConf = server_data_conf/vip_rule.data
routeRuleConf = server_data_conf/route_rule.data
clusterConf = server_data_conf/cluster_conf.data

# load balancing related conf
gslbConf = cluster_conf/gslb.data
clusterTableConf = cluster_conf/cluster_table.data

# naming related conf
nameConf = server_data_conf/name_conf.data

# moduels enabled
modules = mod_trust_clientip
modules = mod_block
modules = mod_header
modules = mod_rewrite
modules = mod_redirect
modules = mod_logid

# interval for get diff of proxy-state
monitorInterval = 20

# debug flags
debugServHttp = false
debugBfeRoute = false
debugBal = false
debugHealthCheck = false

[httpsBasic]
# tls cert conf
serverCertConf = tls_conf/server_cert_conf.data

# tls rule
tlsRuleConf = tls_conf/tls_rule_conf.data

# supported cipherSuites preference settings
#
# ciphersuites implemented in golang:
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

# base directory of client ca certificates
# Note: filename suffix of ca certificate file should be ".crt"
clientCABaseDir = tls_conf/client_ca

[sessionCache]
# disable tls session cache or not
sessionCacheDisabled = true

# address of cache server
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
