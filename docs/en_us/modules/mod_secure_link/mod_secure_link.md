# mod_secure_link

## Introduction

mod_secure_link is used to check authenticity of requested links, protect resources from unauthorized access, and limit link lifetime.

## Module Configuration

- conf/mod_secure_link/mod_secure_link.conf: See [mod_secure_link Basic Configuration](../../configuration/mod_secure_link/mod_secure_link.conf.md).

## Rule Configuration

- conf/mod_secure_link/secure_link_rule.data: See [mod_secure_link Rule Configuration](../../configuration/mod_secure_link/secure_link_rule.data.md).

## Metrics

| Metric | Description |
| ------ | ----------- |
| REQ_TOTAL | Total count of requests |
| REQ_ACCEPT | Count of accepted requests |
| REQ_WITHOUT_EXPIRES_KEY | Count of requests missing expires key |
| REQ_INVALID_EXPIRES_VALUE | Count of requests with invalid expires value |
| REQ_WITHOUT_CHECKSUM_KEY | Count of requests missing checksum key |
| REQ_INVALID_CHECKSUM | Count of requests with invalid checksum |
| REQ_EXPIRED | Count of expired requests |
