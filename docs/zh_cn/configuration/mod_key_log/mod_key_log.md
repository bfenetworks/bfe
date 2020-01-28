# 简介

ModuleKeyLog以NSS key log格式记录TLS会话密钥, 便于基于第三方工具（例如wireshark) 解密分析TLS加密流量，方便问题诊断分析

关于NSS key log详细格式说明，参见:
https://developer.mozilla.org/en-US/docs/Mozilla/Projects/NSS/Key_Log_Format

# 配置

- Module config file

  conf/mod_key_log/mod_key_log.conf

  ```
  [Log]
  # filename prefix for log 
  LogPrefix = key

  # log directory 
  LogDir = ./log

  # interval to rotate logs: M/H/D
  # - M: minute
  # - H: hour
  # - D: day
  RotateWhen = H 

  # max number of rotated log files
  BackupCount = 3

  ```

