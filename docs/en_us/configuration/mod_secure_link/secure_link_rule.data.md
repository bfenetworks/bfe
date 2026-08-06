# mod_secure_link Rule Configuration

## Configuration Introduction

`secure_link_rule.data` is the rule configuration file for the `mod_secure_link` module.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | Usually a timestamp, e.g., `2019-12-10184356` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Rules for each product | Y | Key is product name | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Array | A list of rules | Y | - | - |
| Config{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| Config{v}[].ChecksumKey | String | The key which stored Checksum Value in Query | Y | - | - |
| Config{v}[].ExpiresKey | String | The key which stored Expired time in Query | Y | - | - |
| Config{v}[].ExpressionNodes | Array | Nodes which join calculate Checksum | Y | - | - |
| Config{v}[].ExpressionNodes[].Type | String | Node Type | Y | See Node Type for valid values | - |
| Config{v}[].ExpressionNodes[].Param | String | The param may be used to get Final Value | Conditional | Required when Type is `label`, `query`, or `header` | - |

## Node Type

Supported node types and calculate logic:

| Type | Calculate Logic |
| ---- | --------------- |
| label | $Param |
| query | req.URL.Query($Param) |
| header | req.Header.Get($Param) |
| host | req.Host |
| uri | req.RequestURI |
| remote_addr | req.RemoteAddr |

## Configuration Example

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

## Link Generate Logic

With above config, the pseudo code to generate link is:

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

Step2 sign logic in shell:

```
echo -n $origin | openssl md5 -binary | openssl base64 | tr +/ -_ | tr -d =

// one example:
echo -n '2147483647/s/link127.0.0.1 secret' | openssl md5 -binary | openssl base64 | tr +/ -_ | tr -d =
_e4Nc3iduzkWRm01TBBNYw
```
