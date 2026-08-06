# mod_prison Rule Configuration

## Introduction

`prison.data` is the rule configuration file of `mod_prison`.

## Configuration

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp string, e.g., `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Prison rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | A ordered list of prison rules | Y | - | - |
| Config{v}[] | Object | A prison rule | Y | - | - |
| Config{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) | - |
| Config{v}[].AccessSignConf | Object | Conf of access sign which is the basis for judgment of same access | Y | - | - |
| Config{v}[].AccessSignConf.UseSocketIP | Boolean | Whether using socket ip to generate access sign | N | - | - |
| Config{v}[].AccessSignConf.UseClientIP | Boolean | Whether using client ip to generate access sign | N | - | - |
| Config{v}[].AccessSignConf.UseConnectID | Boolean | Whether using connect id to generate access sign | N | - | - |
| Config{v}[].AccessSignConf.UseUrl | Boolean | Whether using url to generate access sign | N | - | - |
| Config{v}[].AccessSignConf.UseHost | Boolean | Whether using host to generate access sign | N | - | - |
| Config{v}[].AccessSignConf.UsePath | Boolean | Whether using path to generate access sign | N | - | - |
| Config{v}[].AccessSignConf.UseHeaders | Boolean | Whether using headers to generate access sign | N | - | - |
| Config{v}[].AccessSignConf.UrlRegexp | String | Substrings in url matching UrlRegexp which are used for generating access sign | N | - | - |
| Config{v}[].AccessSignConf.[]Query | Array | Query keys used for generating access sign | N | Element type is String | - |
| Config{v}[].AccessSignConf.[]Header | Array | Header keys used for generating access sign | N | Element type is String | - |
| Config{v}[].AccessSignConf.[]Cookie | Array | Cookie keys used for generating access sign | N | Element type is String | - |
| Config{v}[].Action | Object | Prison action if visits exceed the limit | Y | - | - |
| Config{v}[].Action.Cmd | String | Name of prison action | Y | Valid values see module actions | - |
| Config{v}[].Action.Params | Array | Parameters of prison action | N | - | - |
| Config{v}[].CheckPeriod | Integer | Period of check time (second) | Y | - | Positive integer |
| Config{v}[].StayPeriod | Integer | Period of prison time if visits exceed the limit (second) | Y | - | Positive integer |
| Config{v}[].Threshold | Integer | Take action if exceeding threshold during specified CheckPeriod | Y | - | Positive integer |
| Config{v}[].AccessDictSize | Integer | Size of LRU cache for access records | Y | - | Positive integer |
| Config{v}[].PrisonDictSize | Integer | Size of LRU cache for prison records | Y | - | Positive integer |

## Actions

| Action | Description |
| ------ | ----------- |
| CLOSE | Close the connection |
| FINISH | Return 403 response and close the connection |
| PASS | Just forward request |
| REQ_HEADER_SET | Set request header |

## Example

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
