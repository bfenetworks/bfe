# mod_secure_link

## Introduction

mod_secure_link is used to check authenticity of requested links, protect resources from unauthorized access, and limit link lifetime.

## Module Configuration

### Description

the basic config in: conf/mod_secure_link/mod_secure_link.conf

| Config Item    | Description                          |
| -------------- | ------------------------------------ |
| Basic.DataPath | String<br>Path of rule configuration |
| Log.OpenDebug  | Boolean<br>Debug flag of module      |

### Example

```ini
[Basic]
DataPath = ./mod_secure_link/secure_link.data

[Log]
OpenDebug = true
```

## Rule Configuration

### Description

conf/mod_secure_link/secure_link_rule.data

| Config Item                         | Description                                                                           |
| ----------------------------------- | ------------------------------------------------------------------------------------- |
| Version                             | String<br>Version of config file                                                      |
| Config                              | Object<br>Rules for each product                                                      |
| Config{k}                           | String<br>Product name                                                                |
| Config{v}                           | Object<br>A list of rules                                                             |
| Config{v}[].Cond                    | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config[v][].ChecksumKey             | String<br>The key which stored Checksum Value in Query                                |
| Config[v][].ExpiresKey              | String<br>The key which stored Expired time in Query                                  |
| Config[v][].ExpressionNodes         | Array<br>Nodes which join calculate Checksum                                           |
| Config[v][].ExpressionNodes[].Type  | String<br>Node Type, see Node Type to get more information                            |
| Config[v][].ExpressionNodes[].Param | String<br>The param may be used to get Final Value                                    |

### Node Type

be supported node type and Calculate logic:

| type        | Calculate logic         |
| ----------- | ---------------------- |
| label       | $Param                 |
| query       | req.URL.Query($Param)  |
| header      | req.Header.Get($Param) |
| host        | req.Host               |
| uri         | req.RequestURI         |
| remote_addr | req.RemoteAddr         |

### Example

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

### Link generate logic

With above config, the pseudo code to generate link is：

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

step2: the sign logic in shell is:

```
echo -n $origin | openssl md5 -binary | openssl base64 | tr +/ -_ | tr -d =

// one example:
echo -n '2147483647/s/link127.0.0.1 secret' | openssl md5 -binary | openssl base64 | tr +/ -_ | tr -d =
_e4Nc3iduzkWRm01TBBNYw
```
