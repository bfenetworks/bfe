# Configuration reload

BFE has a built-in feature of configuration hot-reload. A new configuration file can be reload by sending a reload request.

## Configure monitor port

Set MonitorPort in BFE core configuration file(conf/bfe.conf)

```ini
[Server]
MonitorPort = 8421
```

## How to reload

* Reload APIs only allows to be accessed using localhost（127.0.0.1/::1）and only supports GET requests

```bash
# reload routing configurations
$ curl http://localhost:8421/reload/server_data_conf  
```

* The complete list of reload APIs can be viewed at http://localhost:8421/reload

## Reload APIs

### Basic function

| Function               | Default configuration file   | Reload API |
| ---------------------- | ---------------------------- | ----------------- |
| routing                | server_data_conf/host_rule.data<br>server_data_conf/vip_rule.data<br>server_data_conf/route_rule.data<br>server_data_conf/cluster_conf.data | /reload/server_data_conf |
| balancing              | cluster_conf/cluster_table.data<br>cluster_conf/gslb.data | /reload/gslb_data_conf |
| name conf              | server_data_conf/name_conf.data | /reload/name_conf |
| TLS rule               | tls_conf/server_cert_conf.data<br>tls_conf/tls_rule_conf.data | /reload/tls_conf |
| TLS session ticket key | tls_conf/session_ticket_key.data | /reload/tls_session_ticket_key |

### Module

| Module           | Default configuration file | Reload API |
| ----------------------- | ---------------------------- | ----------------- |
| mod_auth_basic     | mod_auth_basic/auth_basic_rule.data | /reload/mod_auth_basic|
| mod_block | mod_block/block_rules.data<br>mod_block/ip_blocklist.data | /reload/mod_block.product_rule_table<br>/reload/mod_block.global_ip_table |
| mod_compress       | mod_compress/compress_rule.data | /reload/mod_compress |
| mod_errors         | mod_errors/errors_rule.data | /reload/mod_errors |
| mod_geo            | mod_geo/geo.db | /reload/mod_geo |
| mod_header              | mod_header/header_rule.data | /reload/mod_header |
| mod_redirect        | mod_redirect/redirect.data | /reload/mod_redirect |
| mod_rewrite          | mod_rewrite/rewrite.data    | /reload/mod_rewrite |
| mod_static         | mod_static/static_rule.data<br>mod_static/mime_type.data | /reload/mod_static<br>/reload/mod_static.mime_type |
| mod_trust_clientip | mod_trust_clientip/trust_client_ip.data | /reload/mod_trust_clientip |
