# mod_access

## 模块简介

mod_access以指定格式记录请求日志和会话日志。

## 基础配置

### 配置描述

模块配置文件: conf/mod_access/mod_access.conf

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Log.LogFile | String<br>日志文件路径，用来将日志输出到单个文件中（不进行日志切割） |
| Log.LogPrefix            | String<br>日志文件前缀名称 |
| Log.LogDir | String<br>access日志文件目录 |
| Log.RotateWhen | String<br>日志切割时间，支持 M/H/D/MIDNIGHT/NEXTHOUR |
| Log.BackupCount | Integer<br>最大的日志存储数量 |
| Template.RequestTemplate | String<br>请求日志模板 |
| Template.SessionTemplate | String<br>会话日志模板 |

* RequestTemplate/SessionTemplate 中 $开头的代表变量, 支持的变量列表详见"日志变量"章节说明
* RequestTemplate 还支持以下几种内置模板，如配置 RequestTemplate = COMMON打印CLF日志
  * COMMON：Common Log Format; 等价于配置 RequestTemplate = $host - - $request_time \\"$request_line\\" $status_code $res_len
  * COMBINED：Combined Log Format; 等价于配置 RequestTemplate = $host - - $request_time \\"$request_line\\" $status_code $res_len \\"${Referer}req_header\\" \\"${User-Agent}req_header\\"

### 配置示例

#### 将日志保存到指定路径

```ini
[Log]
# filename prefix for log
LogPrefix = access

# access log directory
LogDir =  ../log

# log rotate interval: M/H/D/MIDNIGHT/NEXTHOUR
RotateWhen = NEXTHOUR

# max number of rotated log files
BackupCount = 2

[Template]
# template of request log
RequestTemplate = "REQUEST_LOG $time clientip: $remote_addr serverip: $server_addr host: $host product: $product user_agent: ${User-Agent}req_header status: $status_code error: $error"

# template of session log
SessionTemplate = "SESSION_LOG  $time clientip: $ses_clientip start_time: $ses_start_time end_time: $ses_end_time overhead: $ses_overhead read_total: $ses_read_total write_total: $ses_write_total keepalive_num: $ses_keepalive_num error: $ses_error"
```

#### 将日志输出到标准输出

```ini
[Log]
# file path for log
LogFile = /dev/stdout

[Template]
# template of request log
RequestTemplate = "REQUEST_LOG $time clientip: $remote_addr serverip: $server_addr host: $host product: $product user_agent: ${User-Agent}req_header status: $status_code error: $error"

# template of session log
SessionTemplate = "SESSION_LOG  $time clientip: $ses_clientip start_time: $ses_start_time end_time: $ses_end_time overhead: $ses_overhead read_total: $ses_read_total write_total: $ses_write_total keepalive_num: $ses_keepalive_num error: $ses_error"

```

## 日志变量

### 请求日志变量

| 变量名                | 含义                                        |
| --------------------- | ------------------------------------------- |
| log_id                | 请求日志ID                                  |
| error                 | 请求处理错误                                |
| product               | 产品线名称                                  |
| host                  | 请求Host字段                                |
| url                   | 请求URL信息                                 |
| vip                   | 请求访问VIP                                 |
| is_trust_clientip     | 请求是否来自信任IP                          |
| req_uri               | 请求行URI                                   |
| req_header            | 请求头部, 例${User-Agent}req_header         |
| req_cookie            | 请求Cookie                                  |
| req_nth               | 请求序号（连接上第几个请求)                 |
| req_body_len          | 请求内容长度                                |
| status_code           | 响应状态码                                  |
| res_proto             | 响应HTTP协议版本                            |
| res_header            | 响应头部, 例${Server}req_header             |
| res_cookie            | 响应Set-Cookie                              |
| redirect              | 重定向响应地址                              |
| res_body_len          | 响应内容长度                                |
| remote_addr           | 连接对端地址                                |
| server_addr           | 连接本地地址                                |
| backend               | 请求转发后端信息(集群、子集群、实例)        |
| cluster_name          | 请求转发集群名称                            |
| subcluster            | 请求转发子集群名称                          |
| retry_num             | 请求转发重试次数                            |
| all_time              | 请求总处理时间                              |
| read_req_duration     | 读请求头持续时间                            |
| proxy_delay           | 从接收到请求头到开始转发请求延迟时间        |
| connect_time          | 连接后端时间                                |
| write_serve_time      | 从请求后端到接收到后端响应头持续时间        |
| response_duration     | 从接收到响应头到完成响应转发持续时间        |
| cluster_duration      | 从请求后端到接收到响应头部持续时间（含重试）|
| last_backend_duration | 从请求后端到接收到响应头部持续时间          |
| readwrite_serve_time  | 从请求后端到完成响应转发持续时间            |
| since_ses_start_time  | 接收到请求时当前会话持续时间                |

### 会话日志变量

| 变量名                | 含义                                        |
| --------------------- | ------------------------------------------- |
| ses_clientip          | 会话用户IP                                  |
| ses_error             | 会话错误                                    |
| ses_is_secure         | 是否基于TLS协议                             |
| ses_start_time        | 会话开始时间                                |
| ses_end_time          | 会话结束时间                                |
| ses_overhead          | 会话持续时长                                |
| ses_read_total        | 会话读取字节总数                            |
| ses_write_total       | 会话写出字节总数                            |
| ses_tls_client_random | TLS连接ClientHello Random                   |
| ses_tls_server_random | TLS连接ServerHello Random                   |
| ses_use100            | 是否出现Expect: 100-continue请求            |
| ses_keepalive_num     | 会话总处理请求数                            |

### 通用日志变量

| 变量名                | 含义                                        |
| --------------------- | ------------------------------------------- |
| time                  | 日志记录时间                                |
