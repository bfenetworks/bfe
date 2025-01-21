# mod_auth_jwt

## Introduction

mod_auth_jwt implements JWT([JSON Web Token](https://tools.ietf.org/html/rfc7519)).

## Module Configuration

### Description

conf/mod_auth_jwt/mod_auth_jwt.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.DataPath | String<br>Path of rule configuration |
| Log.OpenDebug | Boolean<br>Debug flag of module |

### Example

```ini
[Basic]
DataPath = mod_auth_jwt/auth_jwt_rule.data
```

## Rule Configuration

### Description

conf/mod_auth_jwt/auth_jwt_rule.data

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of config file |
| Config      | Struct<br>JWT rules for each product |
| Config{k}   | String<br>Product name |
| Config{v}   | Object<br>A ordered list of rules |
| Config{v}[] | Object<br>A rule |
| Config{v}[].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].KeyFile | String<br>Path of JWK configuration |
| Config{v}[].Realm | String<br>Realm, ie. protection space<br>Default "Restricted" |

Description about JWK configuration

* Key file must follow the format described by the [JSON Web Key Specification](https://tools.ietf.org/html/rfc7517)
* Generate key:

```
echo -n jwt_example | base64 | tr '+/' '-_' | tr -d '='
```

* key file configuration example

```json
[
    {
        "k": "and0X2V4YW1wbGU",
        "kty": "oct",
        "kid": "0001"
    }
]
```

### Example

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
