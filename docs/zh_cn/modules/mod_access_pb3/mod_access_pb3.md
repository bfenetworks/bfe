# mod_access_pb3

## 模块简介

mod_access_pb3 用于以 Protocol Buffers v3 格式记录访问日志。

## 基础配置

模块基础配置文件说明详见 [mod_access_pb3.conf](../../configuration/mod_access_pb3/mod_access_pb3.conf.md)。

## 日志说明

mod_access_pb3 以二进制 protobuf 格式输出请求日志和会话日志，日志定义参见 [bfe-access-pb](https://github.com/bfenetworks/bfe-access-pb)。

请求日志标签（log_tag）规则：
- 正常请求：`req_<product name>`
- 错误请求：`req_err_<product name>`
- 错误请求且 product 为空：`req_err_bfe`

## 监控项

| 监控项         | 描述                     |
| -------------- | ------------------------ |
| ALL_REQ_LOG_COUNT | 请求日志总数             |
| ALL_SES_LOG_COUNT | 会话日志总数             |
