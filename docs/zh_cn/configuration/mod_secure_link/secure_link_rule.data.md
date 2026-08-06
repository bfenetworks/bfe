# mod_secure_link 规则配置

## 配置简介

`secure_link_rule.data` 是 `mod_secure_link` 模块的规则配置文件。

## 配置描述

| 配置项                              | 类型   | 参数含义                               | 必填 | 补充描述                                                   | 合法性条件                                           |
| ----------------------------------- | ------ | -------------------------------------- | ---- | ---------------------------------------------------------- | ---------------------------------------------------- |
| Version                             | String | 配置文件版本                           | Y    | 通常采用时间戳格式，如 `2019-12-10184356`                  | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                              | Object | 各产品线的规则配置                     | Y    | 以产品线名称为键                                           | -                                                    |
| Config[k]                           | String | 产品线名称                             | Y    | -                                                          | -                                                    |
| Config[v]                           | Array  | 产品线规则列表                         | Y    | -                                                          | -                                                    |
| Config[v][].Cond                    | String | 规则条件                               | Y    | 语法详见 [Condition](../../condition/condition_grammar.md) | -                                                    |
| Config[v][].ChecksumKey             | String | Query 存放签名结果的 key               | Y    | -                                                          | -                                                    |
| Config[v][].ExpiresKey              | String | Query 存放签名过期时间戳的 key         | Y    | -                                                          | -                                                    |
| Config[v][].ExpressionNodes         | Array  | 参与签名的数据节点列表                 | Y    | -                                                          | -                                                    |
| Config[v][].ExpressionNodes[].Type  | String | 参与签名的数据节点的类型               | Y    | 合法值详见 Node Type 说明                                  | -                                                    |
| Config[v][].ExpressionNodes[].Param | String | 参与签名的数据节点的取值使用的 key     | 条件 | 当 Type 为 `label`、`query`、`header` 时必填               | -                                                    |

## Node Type

当前支持的类型和取值规则有：

| type        | 取值逻辑               |
| ----------- | ---------------------- |
| label       | $Param                 |
| query       | req.URL.Query($Param)  |
| header      | req.Header.Get($Param) |
| host        | req.Host               |
| uri         | req.RequestURI         |
| remote_addr | req.RemoteAddr         |

## 配置示例

```json
{
    "Version": "2019-12-10184356",
	"Config": {
		"p1": [{
			"Cond": "default_t()",
			"ChecksumKey": "sign",
			"ExpiresKey": "time",
			"ExpressionNodes": [{
					"Type": "query",
					"Param": "time"
				},
				{
					"Type": "uri"
				},
				{
					"Type": "remote_addr"
				},
				{
					"Type": "label",
					"Param": " secret"
				}
			]
		}]
	}
}
```

## Link生成逻辑

以上述配置举例，Path的生成逻辑为：

```
func WrapSecureLinkParam (req *http.Request) {
	now := time.Now().Unix()
	expires := now + int64(time.Hour*24/time.Second)
	// step1: get origin data
	origin := fmt.Sprintf("%d%s%s%s", expires, req.RequestURI, req.RemoteAddr, " secret")

	// step2: generator sign
	sign := func(origin string) string {
		tmpB := md5.Sum([]byte(origin))
		tmp := base64.StdEncoding.EncodeToString(tmpB[:])
		tmp = strings.ReplaceAll(tmp, "+", "-")
		tmp = strings.ReplaceAll(tmp, "/", "_")
		tmp = strings.ReplaceAll(tmp, "=", "")
		return tmp
	}

	// step3: generate link
	req.URL.Query().Set("sign", sign(origin))
	req.URL.Query().Set("time", fmt.Sprintf("%d", expires))
}
```

step2 的逻辑用shell命令表示为：

```
echo -n $origin | openssl md5 -binary | openssl base64 | tr +/ -_ | tr -d =

// one example:
echo -n '2147483647/s/link127.0.0.1 secret' | openssl md5 -binary | openssl base64 | tr +/ -_ | tr -d =
_e4Nc3iduzkWRm01TBBNYw
```
