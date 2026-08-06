# 核心配置

## 配置简介

bfe.conf是BFE的核心配置

## 配置描述

### 服务基础配置

| 配置项                         | 类型    | 参数含义                                           | 必填 | 补充描述                                                                 | 合法性条件                                      |
| ------------------------------ | ------- | -------------------------------------------------- | ---- | ------------------------------------------------------------------------ | ----------------------------------------------- |
| Server.HttpPort                | Integer | HTTP监听端口                                       | N    | 默认值8080；参见 [Port](00-common.md#1-网络端口port) 类型定义             | 类型为 [Port](00-common.md#1-网络端口port)，取值范围 [1, 65535]             |
| Server.HttpsPort               | Integer | HTTPS(TLS)监听端口                                 | N    | 默认值8443；参见 [Port](00-common.md#1-网络端口port) 类型定义             | 类型为 [Port](00-common.md#1-网络端口port)，取值范围 [1, 65535]             |
| Server.MonitorPort             | Integer | Monitor监听端口                                    | N    | 默认值8421；参见 [Port](00-common.md#1-网络端口port) 类型定义             | 类型为 [Port](00-common.md#1-网络端口port)；`MonitorEnabled=true` 时取值范围 [1, 65535] |
| Server.MonitorEnabled          | Boolean | Monitor服务器是否开启                              | N    | 默认值`True`                                                             | -                                               |
| Server.MaxCpus                 | Integer | 最大使用CPU核数                                    | N    | 默认值0；0代表使用所有CPU核                                              | >= 0                                            |
| Server.Layer4LoadBalancer      | String  | 四层负载均衡器类型                                 | N    | 默认值`NONE`                                                             | 仅支持 `PROXY` / `NONE`                         |
| Server.TlsHandshakeTimeout     | Integer | TLS握手超时时间，单位为秒                          | N    | 默认值30                                                                 | > 0 且 <= 1200                                  |
| Server.ClientReadTimeout       | Integer | 读客户端超时时间，单位为秒                         | N    | 默认值60                                                                 | > 0                                             |
| Server.ClientWriteTimeout      | Integer | 写客户端超时时间，单位为秒                         | N    | 默认值60                                                                 | > 0                                             |
| Server.GracefulShutdownTimeout | Integer | 优雅退出超时时间，单位为秒                         | N    | 默认值10                                                                 | (0, 300]                                        |
| Server.KeepAliveEnabled        | Boolean | 与用户端连接是否启用HTTP KeepAlive                 | N    | 默认值`True`                                                             | -                                               |
| Server.MaxHeaderBytes          | Integer | 请求头部的最大长度，单位为Byte                     | N    | 默认值1048576                                                            | > 0                                             |
| Server.MaxHeaderUriBytes       | Integer | 请求头部URI的最大长度，单位为Byte                  | N    | 默认值8192                                                               | > 0                                             |
| Server.MaxProxyHeaderBytes     | Integer | PROXY协议头部的最大长度，单位为Byte                | N    | 默认值0                                                                  | >= 0                                            |
| Server.HttpAddr                | String  | HTTP监听地址                                       | N    | 参见 [ListenAddr](00-common.md#2-监听地址listenaddr) 类型定义            | 类型为 [ListenAddr](00-common.md#2-监听地址listenaddr)                     |
| Server.HttpsAddr               | String  | HTTPS监听地址                                      | N    | 参见 [ListenAddr](00-common.md#2-监听地址listenaddr) 类型定义            | 类型为 [ListenAddr](00-common.md#2-监听地址listenaddr)                     |
| Server.MonitorAddr             | String  | Monitor监听地址                                    | N    | 参见 [ListenAddr](00-common.md#2-监听地址listenaddr) 类型定义            | 类型为 [ListenAddr](00-common.md#2-监听地址listenaddr)                     |
| Server.AcceptNum               | Integer | 每个监听地址的Accept协程数                         | N    | 默认值1；为0时自动设为1                                                  | >= 0                                            |
| Server.EnableAiGateway         | Boolean | 是否启用AI Gateway模式                             | N    | 默认值`False`                                                            | -                                               |
| Server.EstimateToken           | Boolean | 是否基于请求Content-Length估算token使用量          | N    | 默认值`False`                                                            | -                                               |
| Server.HostRuleConf            | String  | [租户域名表配置](server_data_conf/host_rule.data.md)文件路径 | N    | 默认值`server_data_conf/host_rule.data`；参见 [FilePath](00-common.md#3-文件路径filepath) 类型定义 | 类型为 [FilePath](00-common.md#3-文件路径filepath)                         |
| Server.VipRuleConf             | String  | [租户VIP表配置](server_data_conf/vip_rule.data.md)文件路径 | N    | 默认值`server_data_conf/vip_rule.data`；参见 [FilePath](00-common.md#3-文件路径filepath) 类型定义 | 类型为 [FilePath](00-common.md#3-文件路径filepath)                         |
| Server.RouteRuleConf           | String  | [转发规则配置](server_data_conf/route_rule.data.md)文件路径 | N    | 默认值`server_data_conf/route_rule.data`；参见 [FilePath](00-common.md#3-文件路径filepath) 类型定义 | 类型为 [FilePath](00-common.md#3-文件路径filepath)                         |
| Server.ClusterConf             | String  | [后端集群相关配置](server_data_conf/cluster_conf.data.md)文件路径 | N    | 默认值`server_data_conf/cluster_conf.data`；参见 [FilePath](00-common.md#3-文件路径filepath) 类型定义 | 类型为 [FilePath](00-common.md#3-文件路径filepath)                         |
| Server.GslbConf                | String  | [子集群级别负载均衡配置](cluster_conf/gslb.data.md)文件(GSLB)路径 | N    | 默认值`cluster_conf/gslb.data`；参见 [FilePath](00-common.md#3-文件路径filepath) 类型定义 | 类型为 [FilePath](00-common.md#3-文件路径filepath)                         |
| Server.ClusterTableConf        | String  | [实例级别负载均衡配置](cluster_conf/cluster_table.data.md)文件路径 | N    | 默认值`cluster_conf/cluster_table.data`；参见 [FilePath](00-common.md#3-文件路径filepath) 类型定义 | 类型为 [FilePath](00-common.md#3-文件路径filepath)                         |
| Server.NameConf                | String  | [名字与实例映射表配置](server_data_conf/name_conf.data.md)文件路径 | N    | 可选配置；未配置时不加载；参见 [FilePath](00-common.md#3-文件路径filepath) 类型定义 | 类型为 [FilePath](00-common.md#3-文件路径filepath)                         |
| Server.Modules                 | String  | 启用的模块列表                                     | N    | 默认值空；启用多个模块请增加多行Modules配置，参见配置示例                | -                                               |
| Server.MonitorInterval         | Integer | Monitor数据统计周期，单位为秒                      | N    | 默认值20；必须能整除60；大于60时会被截断为60                             | [20, 60] 且能整除60                             |
| Server.DebugServHttp           | Boolean | 是否开启反向代理模块调试日志                       | N    | 默认值`False`                                                            | -                                               |
| Server.DebugBfeRoute           | Boolean | 是否开启流量路由模块调试日志                       | N    | 默认值`False`                                                            | -                                               |
| Server.DebugBal                | Boolean | 是否开启负载均衡模块调试日志                       | N    | 默认值`False`                                                            | -                                               |
| Server.DebugHealthCheck        | Boolean | 是否开启健康检查模块调试日志                       | N    | 默认值`False`                                                            | -                                               |

### TLS基础配置

| 配置项                               | 类型      | 参数含义                                                                         | 必填 | 补充描述                                                                 | 合法性条件                                                                 |
| ------------------------------------ | --------- | -------------------------------------------------------------------------------- | ---- | ------------------------------------------------------------------------ | -------------------------------------------------------------------------- |
| HttpsBasic.ServerCertConf            | String    | [服务端证书与密钥的配置](tls_conf/server_cert_conf.data.md)文件路径              | N    | 默认值`tls_conf/server_cert_conf.data`；参见 [FilePath](00-common.md#3-文件路径filepath) 类型定义 | 类型为 [FilePath](00-common.md#3-文件路径filepath)                         |
| HttpsBasic.TlsRuleConf               | String    | [TLS协议参数配置](tls_conf/tls_rule_conf.data.md)文件路径                        | N    | 默认值`tls_conf/tls_rule_conf.data`；参见 [FilePath](00-common.md#3-文件路径filepath) 类型定义 | 类型为 [FilePath](00-common.md#3-文件路径filepath)                         |
| HttpsBasic.CipherSuites              | String[]  | 启用的加密套件列表                                                               | N    | 启用多个套件请增加多行`CipherSuites`配置，等效套件可用`&#124;`分隔，详见示例 | 必须是BFE支持的加密套件，如 `TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256` 等     |
| HttpsBasic.CurvePreferences          | String[]  | 启用的ECC椭圆曲线                                                                | N    | 默认值`CurveP256`                                                        | 仅支持 `CurveP256`、`CurveP384`、`CurveP521`                               |
| HttpsBasic.EnableSslv2ClientHello    | Boolean   | 针对SSLv3协议，启用对SSLv2格式ClientHello的兼容                                  | N    | 默认值`True`                                                             | -                                                                          |
| HttpsBasic.ClientCABaseDir           | String    | 客户端根CA证书基目录                                                             | N    | 默认值`tls_conf/client_ca`；目录下证书文件后缀须为 `.crt`；参见 [DirPath](00-common.md#4-目录路径dirpath) 类型定义 | 类型为 [DirPath](00-common.md#4-目录路径dirpath)                         |
| HttpsBasic.MaxTlsVersion             | String    | 支持的最高TLS版本                                                                | N    | 默认值`VersionTLS12`                                                     | 仅支持 `VersionSSL30`、`VersionTLS10`、`VersionTLS11`、`VersionTLS12`      |
| HttpsBasic.MinTlsVersion             | String    | 支持的最低TLS版本                                                                | N    | 默认值`VersionSSL30`                                                     | 仅支持上述枚举值；且 `MaxTlsVersion >= MinTlsVersion`                      |
| HttpsBasic.ClientCRLBaseDir          | String    | 客户端CRL基目录                                                                  | N    | 默认值`tls_conf/client_crl`；参见 [DirPath](00-common.md#4-目录路径dirpath) 类型定义 | 类型为 [DirPath](00-common.md#4-目录路径dirpath)                         |
| SessionCache.SessionCacheDisabled    | Boolean   | 是否禁用TLS Session Cache机制                                                    | N    | 默认值`True`；为`True`时跳过其他SessionCache相关校验                     | -                                                                          |
| SessionCache.Servers                 | String    | Cache服务的访问地址                                                              | 条件 | `SessionCacheDisabled=false` 时必填                                      | `SessionCacheDisabled=false` 时不能为空；多个地址以逗号`,`分隔             |
| SessionCache.KeyPrefix               | String    | 缓存key前缀                                                                      | N    | 默认值`bfe`                                                              | -                                                                          |
| SessionCache.ConnectTimeout          | Integer   | 连接Cache服务的超时时间，单位毫秒                                                | N    | 默认值50                                                                 | -                                                                          |
| SessionCache.ReadTimeout             | Integer   | 读取Cache服务的超时时间，单位毫秒                                                | 条件 | `SessionCacheDisabled=false` 时必填                                      | `SessionCacheDisabled=false` 时必须 > 0                                    |
| SessionCache.WriteTimeout            | Integer   | 写入Cache服务的超时时间，单位毫秒                                                | 条件 | `SessionCacheDisabled=false` 时必填                                      | `SessionCacheDisabled=false` 时必须 > 0                                    |
| SessionCache.MaxIdle                 | Integer   | 与Cache服务的最大空闲长连接数                                                    | 条件 | `SessionCacheDisabled=false` 时必填                                      | `SessionCacheDisabled=false` 时必须 > 0                                    |
| SessionCache.SessionExpire           | Integer   | 存储在Cache服务中会话信息的过期时间，单位秒                                      | 条件 | `SessionCacheDisabled=false` 时必填                                      | `SessionCacheDisabled=false` 时必须 > 0                                    |
| SessionTicket.SessionTicketsDisabled | Boolean   | 是否禁用TLS Session Ticket                                                       | N    | 默认值`True`；为`True`时跳过其他SessionTicket相关校验                    | -                                                                          |
| SessionTicket.SessionTicketKeyFile   | String    | [Session Ticket Key配置](tls_conf/session_ticket_key.data.md)文件路径            | N    | 默认值`tls_conf/session_ticket_key.data`；参见 [FilePath](00-common.md#3-文件路径filepath) 类型定义 | 类型为 [FilePath](00-common.md#3-文件路径filepath)                         |

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
