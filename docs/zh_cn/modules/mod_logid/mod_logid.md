# mod_logid

## 模块简介

mod_logid用来生成logid。

## 基础配置

在bfe.conf中启用即可，无其它配置。

## 监控项

| 监控项                  | 描述                              |
| ----------------------- | --------------------------------- |
| NO_LOGID_FROM_UPPER_BFE | 来自信任上游且未携带logid的请求数 |

## logid 格式

自 bfenetworks/bfe#1417 起，logid 为**十进制字符串**，取值 `[1, 2^63-1]`；此前为 32 位十六进制串（128 位），
无法被访问日志的 `uint64` `logid` 字段承载，被静默写为 0。访问日志存储同一取值，字符串与字段可无损互转
（`logid_str == FormatUint(logid, 10)`）；63 位上限用于保证取值落在报表库（MySQL / Doris / StarRocks）
的有符号 `BIGINT` 范围内。
