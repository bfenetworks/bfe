# mod_wasmplugin 基础配置

## 配置简介

`mod_wasm.conf` 是 `mod_wasmplugin` 模块的基础配置文件，用于指定 wasm 插件规则配置文件路径及 wasm 插件文件存放目录等。

## 配置描述

| 配置项                | 类型    | 参数含义                       | 必填 | 补充描述                                                     | 合法性条件                                                   |
| --------------------- | ------- | ------------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Basic.DataPath        | String  | wasm 插件规则配置文件路径      | Y    | 默认值为 `mod_wasm/mod_wasm.data`                            | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Basic.WasmPluginPath  | String  | 存放 wasm 插件文件的文件夹路径 | Y    | 默认值为 `mod_wasm`                                          | 类型为 [DirPath](../00-common.md#4-目录路径dirpath)；目录须存在且可读 |
| Log.OpenDebug         | Boolean | 是否开启 debug 日志            | N    | 默认值为 `False`                                             | -                                                            |

## 配置示例

```ini
[Basic]
DataPath = mod_wasm/mod_wasm.data
WasmPluginPath = wasm_plugin/

[Log]
OpenDebug = true
```
