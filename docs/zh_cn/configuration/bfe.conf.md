# 简介

bfe.conf是BFE的核心配置。

# 配置

## 服务基础配置

| 配置项                  | 类型   | 描述                                                         | 默认值                             |
| ----------------------- | ------ | ------------------------------------------------------------ | ---------------------------------- |
| HttpPort                | Int    | HTTP监听端口                                                 | 8080                               |
| HttpsPort               | Int    | HTTPS(TLS)监听端口                                           | 8443                               |
| MonitorPort             | Int    | Monitor监听端口                                              | 8421                               |
| MaxCpus                 | Int    | 最大使用CPU核数; 0代表使用所有CPU核                          | 0                                  |
| Layer4LoadBalancer      | String | 四层负载均衡器类型 (PROXY/BGW/NONE)                          | NONE                                |
| TlsHandshakeTimeout     | Int    | TLS握手超时时间，单位为秒                                    | 30s                                |
| ClientReadTimeout       | Int    | 读客户端超时时间，单位为秒                                   | 60s                                |
| ClientWriteTimeout      | Int    | 写客户端超时时间，单位为秒                                   | 60s                                |
| GracefulShutdownTimeout | Int    | 优雅退出超时时间，单位为秒，最大300秒                        | 10s                                |
| KeepAliveEnabled        | Bool   | 与用户端连接是否启用HTTP KeepAlive                           | 启用                               |
| MaxHeaderBytes          | Int    | 请求头部的最大长度，单位为Byte                               | 1048576                            |
| MaxHeaderUriBytes       | Int    | 请求头部URI的最大长度，单位为Byte                            | 8192                               |
| HostRuleConf            | String | 租户域名表配置文件路径                                       | server_data_conf/host_rule.data    |
| VipRuleConf             | String | 租户VIP表配置文件路径                                        | server_data_conf/vip_rule.data     |
| RouteRuleConf           | String | 转发规则配置文件路径                                         | server_data_conf/route_rule.data   |
| ClusterConf             | String | 后端集群相关配置文件路径                                     | server_data_conf/cluster_conf.data |
| GslbConf                | String | 子集群级别负载均衡配置文件(GSLB)路径                         | cluster_conf/gslb.data             |
| ClusterTableConf        | String | 实例级别负载均衡配置文件路径                                 | cluster_conf/cluster_table.data    |
| NameConf                | String | 名字与实例映射表配置文件                                     | server_data_conf/name_conf.data    |
| Modules                 | String | 启用的模块列表; 启用多个模块请增加多行Modules配置，详见下文示例 | 无                                 |
| MonitorInterval         | Int    | Monitor数据统计周期，单位为秒                                | 20s                                |
| DebugServHttp           | Bool   | 是否开启反向代理模块调试日志                                 | 否                                 |
| DebugBfeRoute           | Bool   | 是否开启流量路由模块调试日志                                 | 否                                 |
| DebugBal                | Bool   | 是否开启负载均衡模块调试日志                                 | 否                                 |
| DebugHealthCheck        | Bool   | 是否开启健康检查模块调试日志                                 | 否                                 |

## TLS基础配置

| 配置项                 | 类型   | 描述                                                         | 默认值                                                       |
| ---------------------- | ------ | ------------------------------------------------------------ | ------------------------------------------------------------ |
| ServerCertConf         | String | 服务端证书与密钥的配置文件路径                               | tls_conf/server_cert_conf.data                               |
| TlsRuleConf            | String | TLS协议参数配置文件路径                                      | tls_conf/tls_rule_conf.data                                  |
| CipherSuites           | String | 启用的加密套件列表; 启用多个套件请增加多行cipherSuites配置，详见下文示例 | TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256&#124;TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256&#124;TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256&#124;TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |
| CurvePreferences       | String | 启用的ECC椭圆曲线 ，详见下文示例                             | CurveP256                                                    |
| EnableSslv2ClientHello | Bool   | 针对SSLv3协议，启用对SSLv2格式ClientHello的兼容              | 启用                                                         |
| ClientCABaseDir        | String | 客户端根CA证书基目录 <br>注意：证书文件后缀约定必须是 ".crt" | tls_conf/client_ca                                           |

## TLS Session Cache相关配置

| 配置项               | 类型   | 描述                                        | 默认值 |
| -------------------- | ------ | ------------------------------------------- | ------ |
| SessionCacheDisabled | Bool   | 是否禁用TLS Session Cache机制               | 禁用   |
| Servers              | String | Cache服务的访问地址                         | 无     |
| KeyPrefix            | String | 缓存key前缀                                 | bfe    |
| ConnectTimeout       | Int    | 连接Cache服务的超时时间, 单位毫秒           | 50ms   |
| ReadTimeout          | Int    | 读取Cache服务的超时时间, 单位毫秒           | 无     |
| WriteTimeout         | Int    | 写入Cache服务的超时时间, 单位毫秒           | 50ms   |
| MaxIdle              | Int    | 与Cache服务的最大空闲长连接数               | 20ms   |
| SessionExpire        | Int    | 存储在Cache服务中会话信息的过期时间, 单位秒 | 3600s  |

## TLS Session Ticket相关配置

| 配置项                 | 类型   | 描述                       | 默认值                           |
| ---------------------- | ------ | -------------------------- | -------------------------------- |
| SessionTicketsDisabled | Bool   | 是否禁用TLS Session Ticket | 禁用                             |
| SessionTicketKeyFile   | String | Session Ticket Key文件路径 | tls_conf/session_ticket_key.data |

# 示例

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
