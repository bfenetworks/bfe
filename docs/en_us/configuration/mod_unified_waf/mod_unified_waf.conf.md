# mod_unified_waf Basic Configuration

## Introduction

`mod_unified_waf.conf` is the basic configuration file of the `mod_unified_waf` module, used to specify the third-party WAF product name, connection pool size, data file paths, etc.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.WafProductName | String | Name of the third-party WAF product | N | Candidates: `None`, `BFEMockWaf`; default is `None` | Must be one of the candidates |
| Basic.ConnPoolSize | Integer | Connection pool size to WAF server | N | Default is `8` | Positive integer |
| ConfigPath.ModWafDataPath | String | Path of WAF access parameter configuration | Y | - | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| ConfigPath.ProductParamPath | String | Path of WAF product configuration | Y | - | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| ConfigPath.WafInstancesPath | String | Path of WAF RS instance pool configuration | Y | - | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default `False` | - |

## Configuration Example

```ini
[Basic]
#candidates: None, BFEMockWaf
WafProductName = None
ConnPoolSize = 8

[ConfigPath]
ModWafDataPath = "../conf/mod_unified_waf/mod_unified_waf.data"
ProductParamPath = "../conf/mod_unified_waf/product_param.data"
WafInstancesPath = "../conf/mod_unified_waf/waf_instances.data"

[Log]
OpenDebug = false
```
