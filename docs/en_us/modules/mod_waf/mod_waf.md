# mod_waf

## Introduction

mod_waf performs WAF (Web Application Firewall) detection on requests based on custom rules. It supports block rules (block directly on hit) and check rules (log only on hit).

## Module Configuration

### Description

Module configuration file: conf/mod_waf/mod_waf.conf

| Config Item | Description |
| ----------- | ----------- |
| Basic.ProductRulePath | String<br>Path of WAF rule file<br>Default mod_waf/waf_rule.data |
| Log.LogFile | String<br>Path of log file; outputs logs to a single file without rotation |
| Log.LogPrefix | String<br>Log file prefix |
| Log.LogDir | String<br>Log file directory |
| Log.RotateWhen | String<br>Log rotation time, e.g. M/H/D/MIDNIGHT/NEXTHOUR |
| Log.BackupCount | Integer<br>Max number of backup log files |

### Example

```ini
[Basic]
ProductRulePath = mod_waf/waf_rule.data

[Log]
LogPrefix = waf
LogDir = ../log
RotateWhen = NEXTHOUR
BackupCount = 24
```

## Rule Configuration

### Description

Rule configuration file: waf_rule.data

| Config Item | Description |
| ----------- | ----------- |
| Version | String<br>Version of config file |
| Config | Object<br>WAF rules for each product |
| Config{k} | String<br>Product name |
| Config{v} | Object<br>List of WAF rules under the product |
| Config{v}[] | Object<br>Detailed WAF rule |
| Config{v}[].Cond | String<br>Condition to match the request, see [Condition](../../condition/condition_grammar.md) |
| Config{v}[].BlockRules | []String<br>List of rule names that block directly on hit |
| Config{v}[].CheckRules | []String<br>List of rule names that log only on hit |

### Supported WAF Rules

| Rule Name | Description |
| --------- | ----------- |
| RuleBashCmd | Bash command injection detection |

### Example

```json
{
    "Version": "2019-12-10184356",
    "Config": {
        "example_product": [
            {
                "Cond": "default_t()",
                "BlockRules": [
                    "RuleBashCmd"
                ]
            }
        ]
    }
}
```

## Metrics

| Metric | Description |
| ------ | ----------- |
| CHECKED_REQ | Count of WAF checked requests |
| HIT_BLOCKED_REQ | Count of requests hitting block rules |
| HIT_CHECKED_RULE | Count of requests hitting check rules |
| BLOCKED_RULE_ERROR | Count of errors executing block rules |
| CHECKED_RULE_ERROR | Count of errors executing check rules |
