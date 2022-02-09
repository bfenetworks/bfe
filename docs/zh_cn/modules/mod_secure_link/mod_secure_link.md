# mod_secure_link

## 模块简介

mod_secure_link 校验请求链接是否授权，保护链接不被未授权访问，同时还限制链接的有效期。

## 基础配置

### 配置描述

模块基础配置文件: conf/mod_secure_link/mod_secure_link.conf

| 配置项         | 描述                             |
| -------------- | -------------------------------- |
| Basic.DataPath | String<br>规则配置文件路径       |
| Log.OpenDebug  | Bool<br>是否启用模块调试日志开关 |

### 配置示例

```ini
[Basic]
DataPath = ./mod_secure_link/secure_link.data

[Log]
OpenDebug = true
```

## 规则配置

### 配置描述

模块规则配置文件：conf/mod_secure_link/secure_link_rule.data

| 配置项                              | 描述                                                                          |
| ----------------------------------- | ----------------------------------------------------------------------------- |
| Version                             | String<br>配置文件版本                                                        |
| Config                              | Object<br>各产品线的规则配置                                                  |
| Config[k]                           | String<br>产品线名称                                                          |
| Config[v]                           | Object<br>产品线规则列表                                                      |
| Config[v][].Cond                    | String<br>规则条件, 语法详见[Condition](../../condition/condition_grammar.md) |
| Config[v][].ChecksumKey             | String<br>Query 存放签名结果的key                                             |
| Config[v][].ExpiresKey              | String<br>Query 存放签名过期时间戳的key                                       |
| Config[v][].ExpressionNodes         | Array<br>参与签名的数据节点列表                                               |
| Config[v][].ExpressionNodes[].Type  | String<br>参与签名的数据节点的类型，参考Node Type                             |
| Config[v][].ExpressionNodes[].Param | String<br>参与签名的数据节点的取值使用的key                                   |

### Node Type

当前支持的类型和取值规则有：

| type        | 取值逻辑               |
| ----------- | ---------------------- |
| label       | $Param                 |
| query       | req.URL.Query($Param)  |
| header      | req.Header.Get($Param) |
| host        | req.Host               |
| uri         | req.RequestURI         |
| remote_addr | req.RemoteAddr         |

### 配置示例

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

### Link生成逻辑

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
