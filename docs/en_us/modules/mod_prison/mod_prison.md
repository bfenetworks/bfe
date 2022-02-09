# mod_prison

## Introduction

mod_prison limits the amount of requests a user can make in a given period of time based on defined rules.

## Module Configuration

### Description

conf/mod_prison/mod_prison.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.ProductRulePath | String<br>path of rule configuration |

### Example

```ini
[Basic]
ProductRulePath = mod_prison/prison.data
```

## Rule Configuration

### Description

conf/mod_prison/prison.data

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of config file |
| Config      | Object<br>Prison rules for each product |
| Config{k}   | String<br>Product name |
| Config{v}   | Array<br>A ordered list of prison rules |
| Config{v}[] | Object<br>A prison rule |
| Config{v}[].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].AccessSignConf | Object<br>Conf of access sign which is the basis for judgment of same access |
| Config{v}[].AccessSignConf.UseSocketIP | Boolean<br>Whether using socket ip to generate access sign |
| Config{v}[].AccessSignConf.UseClientIP | Boolean<br>Whether using client ip to generate access sign |
| Config{v}[].AccessSignConf.UseConnectID | Boolean<br>Whether using connect id to generate access sign |
| Config{v}[].AccessSignConf.UseUrl | Boolean<br>Whether using url to generate access sign |
| Config{v}[].AccessSignConf.UseHost | Boolean<br>Whether using host to generate access sign |
| Config{v}[].AccessSignConf.UsePath | Boolean<br>Whether using path to generate access sign |
| Config{v}[].AccessSignConf.UseHeaders | Boolean<br>Whether using headers to generate access sign |
| Config{v}[].AccessSignConf.UrlRegexp | String<br>Substrings in url matching UrlRegexp which are used for generating access sign |
| Config{v}[].AccessSignConf.[]Qeury | Array<br>Qeury keys used for generating access sign |
| Config{v}[].AccessSignConf.[]Header | Array<br>Header keys used for generating access sign |
| Config{v}[].AccessSignConf.[]Cookie | Array<br>Cookie keys used for generating access sign |
| Config{v}[].Action | Object<br>Prison action if visits exceed the limit |
| Config{v}[].Action.Cmd | String<br>Name of prison action |
| Config{v}[].Action.Params | Array<br>Parameters of prison action |
| Config{v}[].CheckPeriod | Integer<br>Period of check time (second) |
| Config{v}[].StayPeriod | Integer<br>Period of prison time if visits exceed the limit (second) |
| Config{v}[].Threshold | Integer<br>Take action if exceeding threshold during specified CheckPeriod |
| Config{v}[].AccessDictSize | Integer<br>Size of LRU cache for access records |
| Config{v}[].PrisonDictSize | Integer<br>Size of LRU cache for prison records |

### Actions

| Action         | Description                                  |
| -------------- | -------------------------------------------- |
| CLOSE          | Close the connection                         |
| FINISH         | Return 403 response and close the connection |
| PASS           | Just forward request                         |
| REQ_HEADER_SET | Set request header                           |

### Example

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
