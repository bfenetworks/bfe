# mod_key_log

## 模块简介

mod_key_log以NSS key log格式记录TLS会话密钥, 便于基于第三方工具（例如wireshark) 解密分析TLS加密流量，方便问题诊断分析

关于NSS key log详细格式说明，参见:
https://developer.mozilla.org/en-US/docs/Mozilla/Projects/NSS/Key_Log_Format

## 基础配置

模块基础配置文件说明详见 [mod_key_log.conf](../../configuration/mod_key_log/mod_key_log.conf.md)。
