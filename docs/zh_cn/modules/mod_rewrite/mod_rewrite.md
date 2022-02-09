# mod_rewrite

## 模块简介

mod_rewrite根据自定义的条件，修改请求的URI。

## 基础配置

### 配置描述

模块配置文件: conf/mod_rewrite/mod_rewrite.conf

| 配置项         | 描述                               |
| -------------- | ---------------------------------- |
| Basic.DataPath | String<br>规则配置文件路径         |

### 配置示例

```ini
[Basic]
DataPath = mod_rewrite/rewrite.data
```

## 规则配置

### 配置描述

规则配置文件: conf/mod_rewrite/rewrite.data

| 配置项                   | 描述                                                    |
| ------------------------ | ------------------------------------------------------- |
| Version                  | String<br>配置文件版本                                  |
| Config                   | Object<br>各产品线的重写规则列表                        |
| Config{k}                | String<br>产品线名称                                    |
| Config{v}                | Object<br>重写规则列表                                  |
| Config{v}[]              | Object<br>重写规则                                      |
| Config{v}[].Cond         | String<br>规则条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].Action       | Object<br>规则动作                                      |
| Config{v}[].Action.Cmd   | Object<br>规则动作名称                                  |
| Config{v}[].Action.Param | Object<br>规则动作参数列表                              |
| Config{v}[].Last         | Boolean<br>当该项为true时，命中某条规则后，不再向后匹配 |

### 模块动作

| 动作                      | 描述                               |
| ------------------------- | ---------------------------------- |
| HOST_SET_FROM_PATH_PREFIX | 根据path前缀设置host               |
| HOST_SET                  | 设置host                           |
| HOST_SUFFIX_REPLACE       | 替换域名后缀                           |
| PATH_SET                  | 设置path                           |
| PATH_PREFIX_ADD           | 增加path前缀                       |
| PATH_PREFIX_TRIM          | 删除path前缀                       |
| QUERY_ADD                 | 增加query                          |
| QUERY_DEL                 | 删除query                          |
| QUERY_RENAME              | 重命名query                        |
| QUERY_DEL_ALL_EXCEPT      | 删除除指定key外的所有query         |

### 配置示例

```json
{
  "Version": "20190101000000",
  "Config": {
      "example_product": [
          {
              "Cond": "req_path_prefix_in(\"/rewrite\", false)",
              "Actions": [
                  {
                      "Cmd": "PATH_PREFIX_ADD",
                      "Params": [
                          "/bfe/"
                      ]
                  }
              ],
              "Last": true
          }
      ]
  }
}
```
