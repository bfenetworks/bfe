# mod_body_process Basic Configuration

## Introduction

`mod_body_process.conf` is the basic configuration file of the `mod_body_process` module, used to specify the rule configuration file path.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.ProductRulePath | String | Path of the rule configuration file | Y | - | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default `False` | - |

## Configuration Example

```ini
[Basic]
ProductRulePath = mod_body_process/body_process_rule.data

[Log]
OpenDebug = false
```
