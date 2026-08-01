# mod_wasmplugin 规则配置

## 配置简介

`mod_wasm.data` 是 `mod_wasmplugin` 模块的规则配置文件，用于配置 wasm 插件的调用规则及插件元信息。

## 配置描述

### 规则配置

| 配置项                             | 类型   | 参数含义                                        | 必填 | 补充描述                                                   | 合法性条件                                           |
| ---------------------------------- | ------ | ----------------------------------------------- | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version                            | String | 配置文件版本                                    | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| BeforeLocationRules                | Array  | HandleBeforeLocation 回调点的 wasm 插件规则列表 | N    | -                                                          | -                                                    |
| BeforeLocationRules[]              | Object | 一条 wasm 插件规则                              | Y    | -                                                          | -                                                    |
| BeforeLocationRules[].Cond         | String | 匹配请求或连接的条件                            | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| BeforeLocationRules[].PluginList   | Array  | 条件匹配时执行的 wasm 插件列表                  | Y    | -                                                          | -                                                    |
| BeforeLocationRules[].PluginList[] | String | wasm 插件名                                     | Y    | 插件名须在 `PluginMap` 中已定义                            | -                                                    |
| ProductRules                       | Object | 各产品线的 wasm 插件规则列表                    | N    | 以产品线名称为键                                           | -                                                    |
| ProductRules{k}                    | String | 产品线名称                                      | Y    | -                                                          | -                                                    |
| ProductRules{v}                    | Array  | 产品线下的 wasm 插件规则列表                    | Y    | -                                                          | -                                                    |
| ProductRules{v}[]                  | Object | 一条 wasm 插件规则                              | Y    | -                                                          | -                                                    |
| ProductRules{v}[].Cond             | String | 匹配请求或连接的条件                            | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| ProductRules{v}[].PluginList       | Array  | 条件匹配时执行的 wasm 插件列表                  | Y    | -                                                          | -                                                    |
| ProductRules{v}[].PluginList[]     | String | wasm 插件名                                     | Y    | 插件名须在 `PluginMap` 中已定义                            | -                                                    |

### 插件配置

| 配置项                   | 类型    | 参数含义              | 必填 | 补充描述                         | 合法性条件   |
| ------------------------ | ------- | --------------------- | ---- | -------------------------------- | ------------ |
| PluginMap                | Object  | wasm 插件字典         | Y    | 以插件名称为键                   | -            |
| PluginMap{k}             | String  | wasm 插件名           | Y    | -                                | -            |
| PluginMap{v}             | Object  | wasm 插件详细信息     | Y    | -                                | -            |
| PluginMap{v}.Name        | String  | wasm 插件名           | Y    | 须与 `PluginMap{k}` 一致         | -            |
| PluginMap{v}.WasmVersion | String  | wasm 插件文件版本     | Y    | 用于匹配插件的 wasm 文件版本     | -            |
| PluginMap{v}.ConfVersion | String  | wasm 插件配置文件版本 | Y    | 用于匹配插件的自定义配置文件版本 | -            |
| PluginMap{v}.InstanceNum | Integer | wasm 插件运行实例数   | Y    | -                                | 须为非负整数 |

## wasm 插件文件

对于 `PluginMap` 中的任意一个 wasm 插件（名为 `PlugName`），需要预先准备好以下文件，存放于路径：`<WasmPluginPath>`/`PlugName`/。

| 文件名        | 描述                      |
| ------------- | ------------------------- |
| PlugName.wasm | wasm 文件                 |
| PlugName.md5  | PlugName.wasm 的 md5 文件 |
| PlugName.conf | 插件自定义配置文件        |

## 配置示例

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
