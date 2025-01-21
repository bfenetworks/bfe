# Log rotation

## Introdution

As time passes, the size of log files increases and occupies more and more disk space.
BFE has a built-in feature of log file rotation which can automatically rotate log files,
remove old ones and retain the recent ones.

## Description

| Name        | Path           | Rotation Configuration            |
| ----------- | -------------- | --------------------------------- |
| Server Log  | log/bfe.log    | rotate log file at midnight; retain the recent 7 log files |
| Access Log  | log/access.log | [conf/mod_access/mod_access.conf](../modules/mod_access/mod_access.md) |
| TLS Key Log | log/key.log    | [conf/mod_key_log/mod_key_log.conf](../modules/mod_key_log/mod_key_log.md) |
