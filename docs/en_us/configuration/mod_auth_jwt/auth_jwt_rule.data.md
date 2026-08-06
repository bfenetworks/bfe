# mod_auth_jwt Rule Configuration

## Configuration Introduction

`auth_jwt_rule.data` is the rule configuration file for the `mod_auth_jwt` module, used to configure JWT authentication rules for each product.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | - | Type is [Version](../00-common.md#5-version) |
| Config | Object | JWT rules for each product | Y | - | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Object | An ordered list of rules for the product | Y | - | - |
| Config{v}[] | Object | A rule | Y | - | - |
| Config{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| Config{v}[].KeyFile | String | Path of JWK configuration | Y | - | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Config{v}[].Realm | String | Realm, i.e. protection space | N | Default value is `"Restricted"` | - |

## JWK Configuration File

The key file must follow the format described by the [JSON Web Key Specification](https://tools.ietf.org/html/rfc7517).

Generate key:

```
echo -n jwt_example | base64 | tr '+/' '-_' | tr -d '='
```

Key file configuration example:

```json
[
    {
        "k": "and0X2V4YW1wbGU",
        "kty": "oct",
        "kid": "0001"
    }
]
```

## Configuration Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "KeyFile": "mod_auth_jwt/key_file",
                "Realm": "Restricted"
            }
        ]
    }
}
```
