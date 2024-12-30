# mod_wasmplugin

## 模块简介

Bfe 支持在 http request/response 的处理流程中引入用户自定义的 wasm插件 （遵循 proxy-wasm 规范， https://github.com/proxy-wasm/spec）。
mod_wasmplugin 负责运行 wasm插件，并根据自定义规则调用 wasm插件。

## 基础配置

### 配置描述

模块配置文件: conf/mod_wasm/mod_wasm.conf

| 配置项                | 描述                                        |
| ---------------------| ------------------------------------------- |
| Basic.DataPath            | String<br>wasm插件规则配置的文件路径 |
| Basic.WasmPluginPath      | String<br>存放wasm插件文件的文件夹路径 |
| Log.OpenDebug           | Boolean<br>是否开启 debug 日志<br>默认值False |

### 配置示例

```ini
[Basic]
DataPath = mod_wasm/mod_wasm.data
WasmPluginPath=wasm_plugin/
```

## wasm插件规则配置

### 配置描述

| 配置项  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| Version | String<br>配置文件版本 |
| BeforeLocationRules | Object<br>HandleBeforeLocation 回调点的 wasm插件规则列表 |
| BeforeLocationRules[] | Object<br>wasm插件规则详细信息 |
| BeforeLocationRules[].Cond | String<br>描述匹配请求或连接的条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| BeforeLocationRules[].PluginList | Object<br>条件匹配时执行的wasm插件列表 |
| BeforeLocationRules[].PluginList[] | String<br>wasm插件名 |
| ProductRules | Object<br>各产品线的 wasm插件规则列表 |
| ProductRules{k} | String<br>产品线名称 |
| ProductRules{v} | Object<br>产品线下的 wasm插件规则列表 |
| ProductRules{v}[] | Object<br>wasm插件规则详细信息 |
| ProductRules{v}[].Cond | String<br>描述匹配请求或连接的条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| ProductRules{v}[].PluginList | Object<br>条件匹配时执行的wasm插件列表 |
| ProductRules{v}[].PluginList[] | String<br>wasm插件名 |
| PluginMap | Object<br>wasm插件字典 |
| PluginMap{k} | String<br>wasm插件名 |
| PluginMap{v} | Object<br>wasm插件详细信息 |
| PluginMap{v}.Name | String<br>wasm插件名 |
| PluginMap{v}.WasmVersion | String<br>wasm插件文件版本 |
| PluginMap{v}.ConfVersion | String<br>wasm插件配置文件版本 |
| PluginMap{v}.InstanceNum | Integer<br>wasm插件运行实例数 |

### 配置示例

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

## wasm插件文件

对于 PluginMap 中的任意一个 wasm插件（名为`PlugName`），需要预先准备好以下文件，存放于路径： `<WasmPluginPath>`/`PlugName`/
| 文件名  | 描述                                                           |
| ------- | -------------------------------------------------------------- |
| PlugName.wasm | wasm 文件 |
| PlugName.md5 | PlugName.wasm 的 md5 文件 |
| PlugName.conf | 插件自定义配置文件 |
