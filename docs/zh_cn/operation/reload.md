# 热加载配置

BFE内置配置热加载功能，通过请求Reload HTTP API能够加载新的配置文件。

## 配置管理端口

在BFE核心配置文件(conf/bfe.conf)中, 配置MonitorPort

```ini
[Server]
MonitorPort = 8421
```

## 使用方式

* reload接口仅允许使用localhost访问（127.0.0.1/::1）, 仅支持GET请求, 示例:

```bash
# 重加载分流转发配置
$ curl http://localhost:8421/reload/server_data_conf 
```

* 完整的Reload API列表可访问http://localhost:8421/reload查看。详见下文说明。

## 接口说明

### 基础功能

| 功能名称                 | 默认配置文件                | 热加载接口          |
| ----------------------- | ---------------------------- | ----------------- |
| 内容路由                 | server_data_conf/host_rule.data<br>server_data_conf/vip_rule.data<br>server_data_conf/route_rule.data<br>server_data_conf/cluster_conf.data | /reload/server_data_conf |
| 负载均衡                 | cluster_conf/cluster_table.data<br>cluster_conf/gslb.data | /reload/gslb_data_conf |
| 名字解析                 | server_data_conf/name_conf.data | /reload/name_conf |
| TLS规则                 | tls_conf/server_cert_conf.data<br>tls_conf/tls_rule_conf.data | /reload/tls_conf |
| TLS session ticket key  | tls_conf/session_ticket_key.data | /reload/tls_session_ticket_key |

### 扩展模块

| 功能名称                 | 默认配置文件                | 热加载接口          |
| ----------------------- | ---------------------------- | ----------------- |
| mod_auth_basic     | mod_auth_basic/auth_basic_rule.data | /reload/mod_auth_basic|
| mod_auth_jwt | mod_auth_jwt/mod_auth_jwt.conf | /reload/mod_auth_jwt |
| mod_block | mod_block/block_rules.data<br>mod_block/ip_blocklist.data | /reload/mod_block.product_rule_table<br>/reload/mod_block.global_ip_table |
| mod_compress       | mod_compress/compress_rule.data | /reload/mod_compress |
| mod_errors         | mod_errors/errors_rule.data | /reload/mod_errors |
| mod_geo            | mod_geo/geo.db | /reload/mod_geo |
| mod_header              | mod_header/header_rule.data | /reload/mod_header |
| mod_redirect        | mod_redirect/redirect.data | /reload/mod_redirect |
| mod_rewrite          | mod_rewrite/rewrite.data    | /reload/mod_rewrite |
| mod_static         | mod_static/static_rule.data<br>mod_static/mime_type.data | /reload/mod_static<br>/reload/mod_static.mime_type |
| mod_trust_clientip | mod_trust_clientip/trust_client_ip.data | /reload/mod_trust_clientip |
