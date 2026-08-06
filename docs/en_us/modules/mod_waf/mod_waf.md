# mod_waf

## Introduction

mod_waf performs WAF (Web Application Firewall) detection on requests based on custom rules. It supports block rules (block directly on hit) and check rules (log only on hit).

## Module Configuration

- [mod_waf.conf](../../configuration/mod_waf/mod_waf.conf.md)

## Rule Configuration

- [waf_rule.data](../../configuration/mod_waf/waf_rule.data.md)

## Metrics

| Metric | Description |
| ------ | ----------- |
| CHECKED_REQ | Count of WAF checked requests |
| HIT_BLOCKED_REQ | Count of requests hitting block rules |
| HIT_CHECKED_RULE | Count of requests hitting check rules |
| BLOCKED_RULE_ERROR | Count of errors executing block rules |
| CHECKED_RULE_ERROR | Count of errors executing check rules |
