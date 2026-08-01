# mod_auth_jwt

## Introduction

mod_auth_jwt implements JWT([JSON Web Token](https://tools.ietf.org/html/rfc7519)).

## Module Configuration

- [mod_auth_jwt.conf](../../configuration/mod_auth_jwt/mod_auth_jwt.conf.md)

## Rule Configuration

- [auth_jwt_rule.data](../../configuration/mod_auth_jwt/auth_jwt_rule.data.md)

## Metrics

| Metric | Description |
| ------ | ----------- |
| REQ_AUTH_RULE_HIT | Count of requests hitting authentication rule |
| REQ_AUTH_NO_AUTHORIZATION | Count of requests without Authorization header |
| REQ_AUTH_AUTHORIZATION_FORMAT_ERR | Count of requests with malformed Authorization header |
| REQ_AUTH_SUCCESS | Count of authentication successes |
| REQ_AUTH_FAILURE | Count of authentication failures |
