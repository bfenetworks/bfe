# mod_trust_clientip

## 模块简介

mod_trust_clientip基于配置信任IP列表，检查并标识访问用户真实IP是否属于信任IP。

## 基础配置

模块基础配置文件说明详见 [mod_trust_clientip.conf](../../configuration/mod_trust_clientip/mod_trust_clientip.conf.md)。

## 字典配置

模块字典配置文件说明详见 [trust_client_ip.data](../../configuration/mod_trust_clientip/trust_client_ip.data.md)。

## 监控信息

| 监控项                       | 描述                                   |
| ---------------------------- | -------------------------------------- |
| CONN_TOTAL                   | 所有连接数                             |
| CONN_TRUST_CLIENTIP          | 来源于信任地址的连接数                 |
| CONN_ADDR_INTERNAL           | 来源于内部地址的连接数                 |
| CONN_ADDR_INTERNAL_NOT_TRUST | 来源于内部地址但不在信任列表的连接数   |
