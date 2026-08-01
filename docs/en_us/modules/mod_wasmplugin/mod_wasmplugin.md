# mod_wasmplugin

## Introduction

Bfe supports calling user-defined wasm plugins (following the proxy-wasm specification, https://github.com/proxy-wasm/spec) in the processing flow of http request/response.
The mod_wasmplugin module is responsible for running wasm plugins and invoking them according to user-defined rules.。

## Configuration

- [Basic Configuration](../../configuration/mod_wasmplugin/mod_wasm.conf.md)
- [Rule Configuration](../../configuration/mod_wasmplugin/mod_wasm.data.md)

## Wasm Plugin Files

For any wasm plugin (with name `PlugName` for example) in the PluginMap, the following files need to be prepared in advance and stored in the path: `<WasmPluginPath>`/`PlugName`/

| File Name  | Description |
| ------- | -------------------------------------------------------------- |
| PlugName.wasm | wasm file |
| PlugName.md5 | md5 file of PlugName.wasm |
| PlugName.conf | Custom configuration file for the plugin |
