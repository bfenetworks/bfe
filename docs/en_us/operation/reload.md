# Configuration Reload

BFE has built-in reload interfaces to support configuration hot reload. A new configuration file can be reload by sending a reload request.

## Config

Use the same port as the monitor

```
[server]
monitorPort = 8421
```

## How to use

* Reload interface only allows access using localhost（127.0.0.1/::1）
* Reload interface only supports GET requests
  * example：curl http://localhost:8421/reload/server_data_conf  will reload route configurations
* The complete list of reload interfaces can be viewed at http://localhost:8421/reload

## Interface description

### basic function

| function               | default config file          | reload interface  |
| ---------------------- | ---------------------------- | ----------------- |
| routing                | server_data_conf/host_rule.data<br>server_data_conf/vip_rule.data<br>server_data_conf/route_rule.data<br>server_data_conf/cluster_conf.data | /reload/server_data_conf |
| balancing              | cluster_conf/cluster_table.data<br>cluster_conf/gslb.data | /reload/gslb_data_conf |
| name conf              | server_data_conf/name_conf.data | /reload/name_conf |
| TLS rule               | tls_conf/server_cert_conf.data<br>tls_conf/tls_rule_conf.data | /reload/tls_conf |
| TLS session ticket key | tls_conf/session_ticket_key.data | /reload/tls_session_ticket_key |

### extension module

| module           | default config file | reload interface |
| ----------------------- | ---------------------------- | ----------------- |
| mod_auth_basic     | mod_auth_basic/auth_basic_rule.data | /reload/mod_auth_basic|
| mod_block | mod_block/block_rules.data<br>mod_block/ip_blacklist.data | /reload/mod_block.product_rule_table<br>/reload/mod_block.global_ip_table |
| mod_compress       | mod_compress/compress_rule.data | /reload/mod_compress |
| mod_errors         | mod_errors/errors_rule.data | /reload/mod_errors |
| mod_geo            | mod_geo/geo.db | /reload/mod_geo |
| mod_header              | mod_header/header_rule.data | /reload/mod_header |
| mod_redirect        | mod_redirect/redirect.data | /reload/mod_redirect |
| mod_rewrite          | mod_rewrite/rewrite.data    | /reload/mod_rewrite |
| mod_static         | mod_static/static_rule.data<br>mod_static/mime_type.data | /reload/mod_static<br>/reload/mod_static.mime_type |
| mod_trust_clientip | mod_trust_clientip/trust_client_ip.data | /reload/mod_trust_clientip |
