# Log file rotation

## Introdution

As time passes, the size of log files increases and occupies more and more disk space.
BFE has a built-in feature of log file rotation which can automatically rotate log files, 
remove old ones and retain the recent ones.

## Description

| Name        | Path           | Rotation Configuration            |
| ----------- | -------------- | --------------------------------- |
| server log  | log/bfe.log    | rotate log file at midnight; retain the recent 7 log files |
| access log  | log/access.log | [conf/mod_access/mod_access.conf](../configuration/mod_access/mod_access.md) |
| tls key log | log/key.log    | [conf/mod_key_log/mod_key_log.conf](../configuration/mod_key_log/mod_key_log.md) |
