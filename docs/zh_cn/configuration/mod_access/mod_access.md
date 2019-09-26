# 简介 

mod_access模块以特定格式记录请求日志和会话日志。

# 配置

- 模块配置文件

  conf/mod_access/mod_access.conf

  ```
  [Log]
  # filename prefix for log
  LogPrefix = access

  # access log directory
  LogDir =  ./

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
