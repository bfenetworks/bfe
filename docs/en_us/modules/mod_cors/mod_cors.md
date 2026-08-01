# mod_cors

## Introduction

mod_cors support Cross-Origin Resource Sharing

## Module configuration

For module configuration, see [mod_cors.conf](../../configuration/mod_cors/mod_cors.conf.md).

## Rule Configuration

For rule configuration, see [cors_rule.data](../../configuration/mod_cors/cors_rule.data.md).

## Metrics

| Metric | Description |
| ------ | ----------- |
| REQ_CORS_RULE_HIT | Count of requests hitting CORS rule |
| REQ_PRE_FLIGHT_HIT | Count of preflight requests hit |
| REQ_ALLOW_ORIGIN_HIT | Count of requests with allowed origin |
| REQ_NOT_ALLOW_ORIGIN_HIT | Count of requests with disallowed origin |
