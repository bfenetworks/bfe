# mod_prison

## 模块简介

mod_prison根据自定义的条件，限定单位时间用户的访问次数。

## 基础配置

### 配置描述

模块配置文件: conf/mod_prison/mod_prison.conf

| 配置项                | 描述                       |
| --------------------- | -------------------------- |
| Basic.ProductRulePath | String<br>规则配置文件路径 |

### 配置示例

```ini
[Basic]
ProductRulePath = mod_prison/prison.data
```

## 规则配置

### 配置描述

规则配置文件: conf/mod_prison/prison.data

| 配置项                   | 描述                                                         |
| ------------------------ | ------------------------------------------------------------ |
| Version                  | String<br>配置文件版本                                       |
| Config                   | Object<br>各产品线的prison规则列表                           |
| Config{k}                | String<br>产品线名称                                         |
| Config{v}                | Array<br>prison规则列表                                      |
| Config{v}[]              | Object<br>单条prison规则                                     |
| Config{v}[].Cond         | String<br>规则条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config{v}[].AccessSignConf | Object<br>计算请求签名的配置，其中签名被用来确定是否为同类请求 |
| Config{v}[].AccessSignConf.UseSocketIP | Boolean<br>计算请求签名时是否使用SocketIP |
| Config{v}[].AccessSignConf.UseClientIP | Boolean<br>计算请求签名时是否使用ClientIP |
| Config{v}[].AccessSignConf.UseConnectID | Boolean<br>计算请求签名时是否使用ConnectID |
| Config{v}[].AccessSignConf.UseUrl | Boolean<br>计算请求签名时是否使用请求的Url |
| Config{v}[].AccessSignConf.UseHost | Boolean<br>计算请求签名时是否使用host |
| Config{v}[].AccessSignConf.UsePath | Boolean<br>计算请求签名时是否使用请求Path |
| Config{v}[].AccessSignConf.UseHeaders | Boolean<br>计算请求签名时是否使用header |
| Config{v}[].AccessSignConf.UrlRegexp | String<br>计算请求签名时使用URL中匹配了UrlRegexp的子串 |
| Config{v}[].AccessSignConf.[]Qeury | Array<br>计算请求签名时使用的query key |
| Config{v}[].AccessSignConf.[]Header | Array<br>计算请求签名时使用的header key |
| Config{v}[].AccessSignConf.[]Cookie | Array<br>计算请求签名时使用的cookie key |
| Config{v}[].Action | Object<br>规则动作 |
| Config{v}[].Action.Cmd | String<br>规则动作名称  |
| Config{v}[].Action.Params | Array<br>规则动作参数列表 |
| Config{v}[].CheckPeriod | Integer<br>检测周期（秒） |
| Config{v}[].StayPeriod | Integer<br>命中规则后的封禁时长 :  惩罚时长（秒） |
| Config{v}[].Threshold | Integer<br>限流阈值 |
| Config{v}[].AccessDictSize | Integer<br>访问统计表大小 |
| Config{v}[].PrisonDictSize | Integer<br>访问封禁表大小 |

### 模块动作

| 动作                      | 描述                               |
| ------------------------- | ---------------------------------- |
| CLOSE                     | 关闭用户连接                     |
| FINISH                    | 回复403响应并关闭用户连接     |
| PASS                      | 正常转发请求 |
| REQ_HEADER_SET            | 修改请求头部                   |

### 配置示例

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
