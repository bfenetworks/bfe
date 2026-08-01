# mod_wasmplugin

## 模块简介

Bfe 支持在 http request/response 的处理流程中引入用户自定义的 wasm插件 （遵循 proxy-wasm 规范， https://github.com/proxy-wasm/spec）。
mod_wasmplugin 负责运行 wasm插件，并根据自定义规则调用 wasm插件。

## 基础配置

模块基础配置文件说明详见 [mod_wasm.conf](../../configuration/mod_wasmplugin/mod_wasm.conf.md)。

## wasm插件规则配置

模块规则配置文件说明详见 [mod_wasm.data](../../configuration/mod_wasmplugin/mod_wasm.data.md)。

## wasm插件文件

对于 `PluginMap` 中的任意一个 wasm插件（名为`PlugName`），需要预先准备好以下文件，存放于路径： `<WasmPluginPath>`/`PlugName`/
| 文件名  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| PlugName.wasm | wasm 文件 |
| PlugName.md5 | PlugName.wasm 的 md5 文件 |
| PlugName.conf | 插件自定义配置文件 |
