# mod_trust_clientip Basic Configuration

## Configuration Introduction

`mod_trust_clientip.conf` is the basic configuration file for the `mod_trust_clientip` module, used to specify the trusted IP dictionary file path.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration | Y | Default value is `mod_trust_clientip/trust_client_ip.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default value is `false` | - |

## Configuration Example

```ini
[Basic]
DataPath = mod_trust_clientip/trust_client_ip.data

[Log]
OpenDebug = false
```
