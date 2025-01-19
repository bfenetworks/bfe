# mod_wasmplugin

## Introduction

Bfe supports calling user-defined wasm plugins (following the proxy-wasm specification, https://github.com/proxy-wasm/spec) in the processing flow of http request/response.
The mod_wasmplugin module is responsible for running wasm plugins and invoking them according to user-defined rules.。

## Module Configuration

### Description

conf/mod_wasm/mod_wasm.conf

| Config Item                | Description                                        |
| ---------------------| ------------------------------------------- |
| Basic.DataPath            | String<br>Path of rule configuration |
| Basic.WasmPluginPath      | String<br>Folder path for storing wasm plugin files |
| Log.OpenDebug           | Boolean<br>Debug flag of module<br>Default value: `False` |

### Example

```ini
[Basic]
DataPath = mod_wasm/mod_wasm.data
WasmPluginPath=wasm_plugin/
```

## Rule Configuration

### Description

| Config Item                | Description                                        |
| ------- | -------------------------------------------------------------- |
| Version | String<br>Version of config file |
| BeforeLocationRules | Object<br>List of wasm plugin rules for the HandleBeforeLocation callback point |
| BeforeLocationRules[] | Object<br>A rule |
| BeforeLocationRules[].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| BeforeLocationRules[].PluginList | Object<br>List of wasm plugins to invoke when the condition is matched |
| BeforeLocationRules[].PluginList[] | String<br>Name of the wasm plugin |
| ProductRules | Object<br>Wasm plugin rules for each product |
| ProductRules{k} | String<br>Product name |
| ProductRules{v} | Object<br>List of wasm plugin rules |
| ProductRules{v}[] | Object<br>A rule |
| ProductRules{v}[].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| ProductRules{v}[].PluginList | Object<br>List of wasm plugins to invoke when the condition is matched |
| ProductRules{v}[].PluginList[] | String<br>Name of the wasm plugin |
| PluginMap | Object<br>Dictionary of wasm plugins |
| PluginMap{k} | String<br>Name of the wasm plugin |
| PluginMap{v} | Object<br>A wasm plugin |
| PluginMap{v}.Name | String<br>Name of the wasm plugin |
| PluginMap{v}.WasmVersion | String<br>Version of the wasm file |
| PluginMap{v}.ConfVersion | String<br>Version of the configuration file |
| PluginMap{v}.InstanceNum | Integer<br>Number of running instances of the wasm plugin |

### Example

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

## Wasm Plugin Files

For any wasm plugin (with name `PlugName` for example) in the PluginMap, the following files need to be prepared in advance and stored in the path: `<WasmPluginPath>`/`PlugName`/

| File Name  | Description |
| ------- | -------------------------------------------------------------- |
| PlugName.wasm | wasm file |
| PlugName.md5 | md5 file of PlugName.wasm |
| PlugName.conf | Custom configuration file for the plugin |
