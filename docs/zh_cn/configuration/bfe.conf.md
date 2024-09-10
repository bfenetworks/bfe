# 核心配置

## 配置简介

bfe.conf是BFE的核心配置

## 配置描述

### 服务基础配置

| 配置项                         | 描述                              |
| ------------------------------ | --------------------------------- |
| Server.HttpPort                | Integer<br>HTTP监听端口<br>默认值8080 |
| Server.HttpsPort               | Integer<br>HTTPS(TLS)监听端口<br>默认值8443 |
| Server.MonitorPort             | Integer<br>Monitor监听端口<br>默认值8421 |
| Server.MonitorEnabled          | Boolean<br>Monitor服务器是否开启<br>默认值True |
| Server.MaxCpus                 | Integer<br>最大使用CPU核数; 0代表使用所有CPU核<br>默认值0 |
| Server.Layer4LoadBalancer      | String<br>四层负载均衡器类型(PROXY/NONE)<br>默认值NONE |
| Server.TlsHandshakeTimeout     | Integer<br>TLS握手超时时间，单位为秒<br>默认值30 |
| Server.ClientReadTimeout       | Integer<br>读客户端超时时间，单位为秒<br>默认值60 |
| Server.ClientWriteTimeout      | Integer<br>写客户端超时时间，单位为秒<br>默认值60 |
| Server.GracefulShutdownTimeout | Integer<br>优雅退出超时时间，单位为秒，最大300秒<br>默认值10 |
| Server.KeepAliveEnabled        | Boolean<br>与用户端连接是否启用HTTP KeepAlive<br>默认值True |
| Server.MaxHeaderBytes          | Integer<br>请求头部的最大长度，单位为Byte<br>默认值1048576 |
| Server.MaxHeaderUriBytes       | Integer<br>请求头部URI的最大长度，单位为Byte<br>默认值8192 |
| Server.HostRuleConf            | String<br>[租户域名表配置](server_data_conf/host_rule.data.md)文件路径<br>默认值server_data_conf/host_rule.data |
| Server.VipRuleConf             | String<br>[租户VIP表配置](server_data_conf/vip_rule.data.md)文件路径<br>默认值server_data_conf/vip_rule.data |
| Server.RouteRuleConf           | String<br>[转发规则配置](server_data_conf/route_rule.data.md)文件路径<br>默认值server_data_conf/route_rule.data |
| Server.ClusterConf             | String<br>[后端集群相关配置](server_data_conf/cluster_conf.data.md)文件路径<br>默认值server_data_conf/cluster_conf.data |
| Server.GslbConf                | String<br>[子集群级别负载均衡配置](cluster_conf/gslb.data.md)文件(GSLB)路径<br>默认值cluster_conf/gslb.data |
| Server.ClusterTableConf        | String<br>[实例级别负载均衡配置](cluster_conf/cluster_table.data.md)文件路径<br>默认值cluster_conf/cluster_table.data |
| Server.NameConf                | String<br>[名字与实例映射表配置](server_data_conf/name_conf.data.md)文件路径<br>默认值server_data_conf/name_conf.data |
| Server.Modules                 | String<br>启用的模块列表; 启用多个模块请增加多行Modules配置，参见配置示例<br>默认值空 |
| Server.MonitorInterval         | Integer<br>Monitor数据统计周期，单位为秒<br>默认值20 |
| Server.DebugServHttp           | Boolean<br>是否开启反向代理模块调试日志<br>默认值False |
| Server.DebugBfeRoute           | Boolean<br>是否开启流量路由模块调试日志<br>默认值False |
| Server.DebugBal                | Boolean<br>是否开启负载均衡模块调试日志<br>默认值False |
| Server.DebugHealthCheck        | Boolean<br>是否开启健康检查模块调试日志<br>默认值False |

### TLS基础配置

| 配置项                            | 描述                                                         |
| --------------------------------- | ------------------------------------------------------------ |
| HttpsBasic.ServerCertConf         | String<br>[服务端证书与密钥的配置](tls_conf/server_cert_conf.data.md)文件路径<br>默认值tls_conf/server_cert_conf.data |
| HttpsBasic.TlsRuleConf            | String<br>[TLS协议参数配置](tls_conf/tls_rule_conf.data.md)文件路径<br>默认值tls_conf/tls_rule_conf.data |
| HttpsBasic.CipherSuites           | String<br>启用的加密套件列表; 启用多个套件请增加多行cipherSuites配置，详见示例<br>默认值TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256&#124;TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256&#124;TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256&#124;TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256<br>TLS_ECDHE_RSA_WITH_RC4_128_SHA<br>TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA<br>TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA<br>TLS_RSA_WITH_RC4_128_SHA<br>TLS_RSA_WITH_AES_128_CBC_SHA<br>TLS_RSA_WITH_AES_256_CBC_SHA |
| HttpsBasic.CurvePreferences       | String<br>启用的ECC椭圆曲线，详见示例<br> 默认值CurveP256 |
| HttpsBasic.EnableSslv2ClientHello | Boolean<br>针对SSLv3协议，启用对SSLv2格式ClientHello的兼容<br>默认值True |
| HttpsBasic.ClientCABaseDir        | String<br>客户端根CA证书基目录; 注意：证书文件后缀约定必须是 ".crt"<br> 默认值tls_conf/client_ca |
| SessionCache.SessionCacheDisabled | Boolean<br>是否禁用TLS Session Cache机制<br>默认值False |
| SessionCache.Servers              | String<br>Cache服务的访问地址<br>默认值无 |
| SessionCache.KeyPrefix            | String<br>缓存key前缀<br>默认值bfe |
| SessionCache.ConnectTimeout       | Integer<br>连接Cache服务的超时时间, 单位毫秒<br>默认值50 |
| SessionCache.ReadTimeout          | Integer<br>读取Cache服务的超时时间, 单位毫秒<br>默认值50 |
| SessionCache.WriteTimeout         | Integer<br>写入Cache服务的超时时间, 单位毫秒<br>默认值50 |
| SessionCache.MaxIdle              | Integer<br>与Cache服务的最大空闲长连接数<br>默认值20 |
| SessionCache.SessionExpire        | Integer<br>存储在Cache服务中会话信息的过期时间, 单位秒<br>默认值3600 |
| SessionTicket.SessionTicketsDisabled | Boolean<br>是否禁用TLS Session Ticket<br>默认值True|
| SessionTicket.SessionTicketKeyFile   | String<br>[Session Ticket Key配置](tls_conf/session_ticket_key.data.md)文件路径<br>默认值tls_conf/session_ticket_key.data |

## 配置示例

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

# moduels enabled
Modules = mod_trust_clientip
Modules = mod_block
Modules = mod_header
Modules = mod_rewrite
Modules = mod_redirect
Modules = mod_logid

# interval for get diff of proxy-state
MonitorInterval = 20

# debug flags
DebugServHttp = false
DebugBfeRoute = false
DebugBal = false
DebugHealthCheck = false

[HttpsBasic]
# tls cert conf
ServerCertConf = tls_conf/server_cert_conf.data

# tls rule
TlsRuleConf = tls_conf/tls_rule_conf.data

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

# base directory of client ca certificates
# Note: filename suffix of ca certificate file should be ".crt"
ClientCABaseDir = tls_conf/client_ca

[SessionCache]
# disable tls session cache or not
SessionCacheDisabled = true

# address of cache server
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
