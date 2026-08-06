# mod_wasmplugin Rule Configuration

## Configuration Introduction

`mod_wasm.data` is the rule configuration file for the `mod_wasmplugin` module, used to configure the invocation rules and metadata of wasm plugins.

## Configuration Description

### Rule Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `20240101000000` | Type is [Version](../00-common.md#5-version) |
| BeforeLocationRules | Array | List of wasm plugin rules for the HandleBeforeLocation callback point | N | - | - |
| BeforeLocationRules[] | Object | A rule | Y | - | - |
| BeforeLocationRules[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| BeforeLocationRules[].PluginList | Array | List of wasm plugins to invoke when the condition is matched | Y | - | - |
| BeforeLocationRules[].PluginList[] | String | Name of the wasm plugin | Y | Plugin name must be defined in `PluginMap` | - |
| ProductRules | Object | Wasm plugin rules for each product | N | Key is product name | - |
| ProductRules{k} | String | Product name | Y | - | - |
| ProductRules{v} | Array | List of wasm plugin rules for the product | Y | - | - |
| ProductRules{v}[] | Object | A rule | Y | - | - |
| ProductRules{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| ProductRules{v}[].PluginList | Array | List of wasm plugins to invoke when the condition is matched | Y | - | - |
| ProductRules{v}[].PluginList[] | String | Name of the wasm plugin | Y | Plugin name must be defined in `PluginMap` | - |

### Plugin Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| PluginMap | Object | Dictionary of wasm plugins | Y | Key is plugin name | - |
| PluginMap{k} | String | Name of the wasm plugin | Y | - | - |
| PluginMap{v} | Object | Detailed information of the wasm plugin | Y | - | - |
| PluginMap{v}.Name | String | Name of the wasm plugin | Y | Must be consistent with `PluginMap{k}` | - |
| PluginMap{v}.WasmVersion | String | Version of the wasm file | Y | Used to match the wasm file version of the plugin | - |
| PluginMap{v}.ConfVersion | String | Version of the configuration file | Y | Used to match the custom configuration file version of the plugin | - |
| PluginMap{v}.InstanceNum | Integer | Number of running instances of the wasm plugin | Y | - | Must be a non-negative integer |

## Wasm Plugin Files

For any wasm plugin (named `PlugName` for example) in `PluginMap`, the following files need to be prepared in advance and stored in the path: `<WasmPluginPath>`/`PlugName/`.

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| PlugName.wasm | File | wasm file | Y | - | - |
| PlugName.md5 | File | md5 file of PlugName.wasm | Y | - | - |
| PlugName.conf | File | Custom configuration file for the plugin | Y | - | - |

## Configuration Example

```json
{
    "Version": "20240101000000",
    "BeforeLocationRules": [{
        "Cond": "req_path_prefix_in(\"/headers\", false)",
        "PluginList": [ "headers" ]
    }],
    "ProductRules": {
        "local_product": [{
            "Cond": "default_t()",
            "PluginList": []
        }]
    },
    "PluginMap": {
        "headers": {
            "Name": "headers",
            "WasmVersion": "20240101000000",
            "ConfVersion": "20240101000000",
            "InstanceNum": 20
        }
    }
}
```
