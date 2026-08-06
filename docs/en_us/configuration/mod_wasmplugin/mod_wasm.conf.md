# mod_wasmplugin Basic Configuration

## Configuration Introduction

`mod_wasm.conf` is the basic configuration file for the `mod_wasmplugin` module, used to specify the wasm plugin rule configuration file path and the folder path for storing wasm plugin files.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration | Y | Default value is `mod_wasm/mod_wasm.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Basic.WasmPluginPath | String | Folder path for storing wasm plugin files | Y | Default value is `mod_wasm` | Type is [DirPath](../00-common.md#4-dirpath); the directory must exist and be readable |
| Log.OpenDebug | Boolean | Debug flag of module | N | Default value is `false` | - |

## Configuration Example

```ini
[Basic]
DataPath = mod_wasm/mod_wasm.data
WasmPluginPath = wasm_plugin/

[Log]
OpenDebug = true
```
