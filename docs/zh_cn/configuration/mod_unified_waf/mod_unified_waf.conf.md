# mod_unified_waf 基础配置

## 配置简介

`mod_unified_waf.conf` 是 `mod_unified_waf` 模块的基础配置文件，用于指定第三方 WAF 产品名称、连接池大小、数据文件路径等。

## 配置描述

| 配置项                | 类型    | 参数含义                 | 必填 | 补充描述                                                     | 合法性条件                                                   |
| --------------------- | ------- | ------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Basic.WafProductName  | String  | 第三方 WAF 产品的名字    | N    | 候选值：None、BFEMockWaf；默认值为 None                      | 须为候选值之一                                               |
| Basic.ConnPoolSize    | Integer | 与 WAF server 的连接池大小 | N    | 默认值为 8                                                   | 正整数                                                       |
| ConfigPath.ModWafDataPath   | String  | WAF 访问具体参数配置文件路径 | Y    | -                                                            | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| ConfigPath.ProductParamPath | String  | WAF 访问产品线配置文件路径 | Y    | -                                                            | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| ConfigPath.WafInstancesPath | String  | WAF RS 实例池配置文件路径 | Y    | -                                                            | 类型为 [FilePath](../00-common.md#3-文件路径filepath)；文件须存在且可读 |
| Log.OpenDebug         | Boolean | 是否开启 debug 日志      | N    | 默认值为 False                                               | -                                                            |

## 配置示例

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
