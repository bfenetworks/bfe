# mod_block

## 模块简介

mod_block基于自定义的规则，对连接或请求进行封禁。

## 基础配置

模块基础配置文件说明详见 [mod_block.conf](../../configuration/mod_block/mod_block.conf.md)。

全局 IP 黑名单文件说明详见 [ip_blocklist.data](../../configuration/mod_block/ip_blocklist.data.md)。

## 规则配置

模块规则配置文件说明详见 [block_rules.data](../../configuration/mod_block/block_rules.data.md)。

## 监控项

| 监控项        | 描述                         |
| ------------- | ---------------------------- |
| CONN_TOTAL    | 连接总数                     |
| CONN_REFUSE   | 连接拒绝的总数               |
| CONN_ACCEPT   | 连接接受的总数               |
| REQ_TOTAL     | 请求总数                     |
| REQ_REFUSE    | 请求拒绝的总数               |
| REQ_ACCEPT    | 请求接受的总数               |
| WRONG_COMMAND | 命中条件但指令非法的请求数   |
