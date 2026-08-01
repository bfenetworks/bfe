# mod_ai_route Basic Configuration

## Introduction

`mod_ai_route.conf` is the basic configuration file of the `mod_ai_route` module, used to specify the AI routing rule file path.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.RouteRulePath | String | Path of the AI routing rule file | Y | - | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default `False` | - |

## Configuration Example

```ini
[Basic]
RouteRulePath = ../data/mod_ai_route/ai_route.data

[Log]
OpenDebug = true
```
