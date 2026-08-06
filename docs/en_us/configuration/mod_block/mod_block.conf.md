# mod_block Basic Configuration

## Introduction

`mod_block.conf` is the basic configuration file of the `mod_block` module, used to specify the block rule file path and the global IP blocklist file path.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.ProductRulePath | String | Path of the product rule configuration file | Y | - | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Basic.IPBlocklistPath | String | Path of the global IP blocklist file | Y | - | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Log.OpenDebug | Boolean | Whether to enable debug logs | N | Default `False` | - |

## Configuration Example

```ini
[Basic]
# product rule config file path
ProductRulePath = mod_block/block_rules.data

# global ip blocklist file path
IPBlocklistPath = mod_block/ip_blocklist.data

[Log]
OpenDebug = false
```
