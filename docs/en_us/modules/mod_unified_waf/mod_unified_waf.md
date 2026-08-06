# mod_unified_waf

## Introduction

BFE supports integrating unified third-party WAF into the HTTP request processing flow.

## Module Configuration

- [mod_unified_waf.conf](../../configuration/mod_unified_waf/mod_unified_waf.conf.md)

## WAF Access Parameter Configuration

- [mod_unified_waf.data](../../configuration/mod_unified_waf/mod_unified_waf.data.md)

## WAF Product Configuration

- [product_param.data](../../configuration/mod_unified_waf/product_param.data.md)

## WAF RS Instance Pool Configuration

- [waf_instances.data](../../configuration/mod_unified_waf/waf_instances.data.md)

## Metrics

| Metric | Description |
| ------ | ----------- |
| REQ_NO_CHECK | Count of requests not checked by WAF |
| REQ_FORBIDDEN | Count of requests blocked by WAF |
| REQ_OK | Count of requests judged normal by WAF |
| REQ_TIMEOUT | Count of WAF detection timeouts |
| REQ_OTHER | Count of other status requests |
| NET_ERR | Count of WAF detection network errors |

The module also records the following delays via delay counter (key prefix):

| Delay Metric | Description |
| ------------ | ----------- |
| waf_client_delay | WAF request-response delay |
| waf_client_delay_peek_body | Body read delay |
| waf_client_delay_call_competition | Concurrency competition delay |
