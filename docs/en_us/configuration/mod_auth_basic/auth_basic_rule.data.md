# mod_auth_basic Rule Configuration

## Configuration Introduction

`auth_basic_rule.data` is the rule configuration file for the `mod_auth_basic` module, used to configure HTTP basic authentication rules for each product.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of config file | Y | - | Type is [Version](../00-common.md#5-version) |
| Config | Object | Auth rules for each product | Y | - | - |
| Config{k} | String | Product name | Y | - | - |
| Config{v} | Object | A list of auth rules for the product | Y | - | - |
| Config{v}[] | Object | An auth rule | Y | - | - |
| Config{v}[].Cond | String | Condition expression | Y | See [Condition](../../condition/condition_grammar.md) for syntax | - |
| Config{v}[].UserFile | String | Path of password configuration | Y | - | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Config{v}[].Realm | String | Realm, i.e. protection space | N | Default value is `"Restricted"` | - |

## Password Configuration File

Passwords are hashed using MD5, SHA1, or BCrypt. The userfile can be generated using htpasswd or openssl.

Generate with openssl:

```
printf "user1:$(openssl passwd -apr1 123456)\n" >> ./userfile
```

Password configuration example:

```  
# user1, 123456
user1:$apr1$mI7SilJz$CWwYJyYKbhVDNl26sdUSh/
user2:{SHA}fEqNCco3Yq9h5ZUglD3CZJT4lBs=:user2, 123456
```

## Configuration Example

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "UserFile": "../conf/mod_auth_basic/userfile",
                "Realm": "example_product"
            }
        ]
    },
    "Version": "20190101000000"
}
```
