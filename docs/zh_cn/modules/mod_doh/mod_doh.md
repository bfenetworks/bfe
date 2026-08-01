# mod_doh

## 模块简介

mod_doh支持DNS over HTTPS。

## 基础配置

模块基础配置文件说明详见 [mod_doh.conf](../../configuration/mod_doh/mod_doh.conf.md)。

## 监控项

| 监控项               | 描述                     |
| -------------------- | ------------------------ |
| DOH_REQUEST          | DoH 请求总数             |
| DOH_REQUEST_NOT_SECURE | 非安全请求的 DoH 请求数  |
| FETCH_DNS_ERR        | DNS 查询失败次数         |
