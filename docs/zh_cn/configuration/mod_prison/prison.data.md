# mod_prison 规则配置

## 配置简介

`prison.data` 是 `mod_prison` 模块的规则配置文件。

## 配置描述

| 配置项                                   | 类型    | 参数含义                                         | 必填 | 补充描述                                                   | 合法性条件                                           |
| ---------------------------------------- | ------- | ------------------------------------------------ | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version                                  | String  | 配置文件版本                                     | Y    | 通常采用时间戳格式，如 `20190101000000`                    | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                                   | Object  | 各产品线的 prison 规则列表                       | Y    | 以产品线名称为键                                           | -                                                    |
| Config{k}                                | String  | 产品线名称                                       | Y    | -                                                          | -                                                    |
| Config{v}                                | Array   | prison 规则列表                                  | Y    | -                                                          | -                                                    |
| Config{v}[]                              | Object  | 单条 prison 规则                                 | Y    | -                                                          | -                                                    |
| Config{v}[].Cond                         | String  | 规则条件                                         | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config{v}[].AccessSignConf               | Object  | 计算请求签名的配置，其中签名被用来确定是否为同类请求 | Y    | -                                                          | -                                                    |
| Config{v}[].AccessSignConf.UseSocketIP   | Boolean | 计算请求签名时是否使用 SocketIP                  | N    | -                                                          | -                                                    |
| Config{v}[].AccessSignConf.UseClientIP   | Boolean | 计算请求签名时是否使用 ClientIP                  | N    | -                                                          | -                                                    |
| Config{v}[].AccessSignConf.UseConnectID  | Boolean | 计算请求签名时是否使用 ConnectID                 | N    | -                                                          | -                                                    |
| Config{v}[].AccessSignConf.UseUrl        | Boolean | 计算请求签名时是否使用请求的 Url                 | N    | -                                                          | -                                                    |
| Config{v}[].AccessSignConf.UseHost       | Boolean | 计算请求签名时是否使用 host                      | N    | -                                                          | -                                                    |
| Config{v}[].AccessSignConf.UsePath       | Boolean | 计算请求签名时是否使用请求 Path                  | N    | -                                                          | -                                                    |
| Config{v}[].AccessSignConf.UseHeaders    | Boolean | 计算请求签名时是否使用 header                    | N    | -                                                          | -                                                    |
| Config{v}[].AccessSignConf.UrlRegexp     | String  | 计算请求签名时使用 URL 中匹配了 UrlRegexp 的子串 | N    | -                                                          | -                                                    |
| Config{v}[].AccessSignConf.[]Query       | Array   | 计算请求签名时使用的 query key                   | N    | 元素类型为 String                                          | -                                                    |
| Config{v}[].AccessSignConf.[]Header      | Array   | 计算请求签名时使用的 header key                  | N    | 元素类型为 String                                          | -                                                    |
| Config{v}[].AccessSignConf.[]Cookie      | Array   | 计算请求签名时使用的 cookie key                  | N    | 元素类型为 String                                          | -                                                    |
| Config{v}[].Action                       | Object  | 规则动作                                         | Y    | -                                                          | -                                                    |
| Config{v}[].Action.Cmd                   | String  | 规则动作名称                                     | Y    | 合法值详见模块动作说明                                     | -                                                    |
| Config{v}[].Action.Params                | Array   | 规则动作参数列表                                 | N    | -                                                          | -                                                    |
| Config{v}[].CheckPeriod                  | Integer | 检测周期                                         | Y    | 单位为秒                                                   | 正整数                                               |
| Config{v}[].StayPeriod                   | Integer | 命中规则后的封禁时长（惩罚时长）                 | Y    | 单位为秒                                                   | 正整数                                               |
| Config{v}[].Threshold                    | Integer | 限流阈值                                         | Y    | -                                                          | 正整数                                               |
| Config{v}[].AccessDictSize               | Integer | 访问统计表大小                                   | Y    | -                                                          | 正整数                                               |
| Config{v}[].PrisonDictSize               | Integer | 访问封禁表大小                                   | Y    | -                                                          | 正整数                                               |

## 模块动作

| 动作                      | 描述                               |
| ------------------------- | ---------------------------------- |
| CLOSE                     | 关闭用户连接                     |
| FINISH                    | 回复403响应并关闭用户连接     |
| PASS                      | 正常转发请求 |
| REQ_HEADER_SET            | 修改请求头部                   |

## 配置示例

```json
{
	"Version": "20190101000000",
	"Config": {
		"example_product": [{
			"Name": "example_prison",
			"Cond": "req_path_prefix_in(\"/prison\", false)",
			"accessSignConf": {
				"url": false,
				"path": false,
				"query": [],
				"header": [],
				"Cookie": [
					"UID"
				]
			},
			"action": {
				"cmd": "CLOSE",
				"params": []
			},
			"checkPeriod": 10,
			"stayPeriod": 10,
			"threshold": 5,
			"accessDictSize": 1000,
			"prisonDictSize": 1000
		}]
	}
}
```
