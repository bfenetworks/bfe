# mod_rewrite 规则配置

## 配置简介

`rewrite.data` 是 `mod_rewrite` 模块的规则配置文件。

## 配置描述

| 配置项                       | 类型    | 参数含义                                         | 必填 | 补充描述                                                   | 合法性条件                                           |
| ---------------------------- | ------- | ------------------------------------------------ | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version                      | String  | 配置文件版本                                     | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                       | Object  | 各产品线的重写规则列表                           | Y    | 以产品线名称为键                                           | -                                                    |
| Config{k}                    | String  | 产品线名称                                       | Y    | -                                                          | -                                                    |
| Config{v}                    | Array   | 重写规则列表                                     | Y    | -                                                          | -                                                    |
| Config{v}[]                  | Object  | 重写规则                                         | Y    | -                                                          | -                                                    |
| Config{v}[].Cond             | String  | 规则条件                                         | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config{v}[].Actions          | Array   | 规则动作列表                                     | Y    | -                                                          | -                                                    |
| Config{v}[].Actions[].Cmd    | String  | 规则动作名称                                     | Y    | 合法值详见模块动作说明                                     | -                                                    |
| Config{v}[].Actions[].Params | Object  | 规则动作参数列表                                 | N    | 依具体动作而定                                             | -                                                    |
| Config{v}[].Last             | Boolean | 当该项为 true 时，命中某条规则后，不再向后匹配   | N    | 默认值为 `false`                                           | -                                                    |

## 模块动作

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

## 配置示例

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
