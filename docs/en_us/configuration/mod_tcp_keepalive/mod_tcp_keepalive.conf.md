# mod_tcp_keepalive Basic Configuration

## Configuration Introduction

`mod_tcp_keepalive.conf` is the basic configuration file for the `mod_tcp_keepalive` module, used to specify the rule configuration file path and log configuration.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of product rule configuration | Y | - | Type is [FilePath](../00-common.md#3-filepath) |
| Log.OpenDebug | Boolean | Open debug mode or not | N | Default value is `false` | - |

## Configuration Example

```ini
[Basic]
DataPath = ../data/mod_tcp_keepalive/tcp_keepalive.data

[Log]
OpenDebug = false
```
