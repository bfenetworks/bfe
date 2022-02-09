# mod_auth_basic

## Introduction

mod_auth_basic implements the HTTP basic authentication.

## Module Configuration

### Description

conf/mod_auth_basic/mod_auth_basic.conf

| Config Item         | Description                                 |
| ------------------- | ------------------------------------------- |
| Basic.DataPath      | String<br>Path of rule configuration |
| Log.OpenDebug       | Boolean<br>Whether enable debug log<br>Default False |

### Example

```ini
[Basic]
DataPath = mod_auth_basic/auth_basic_rule.data

[Log]
OpenDebug = false
```

## Rule Configuration

### Description

| Config Item          | Description                                 |
| ---------------------| ------------------------------------------- |
| Version | String<br>Version of config file |
| Config | Object<br>Auth rules for each product |
| Config{k} | String<br>Product name |
| Config{v} | Object<br> A list of auth rules |
| Config{v}[] | Object<br> A auth rule |
| Config{v}[].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].UserFile | String<br>Path of password configuration |
| Config{v}[].Realm | String<br>Realm, ie. protection space<br>Default "Restricted" |

Description about password configuration:

* The password configuration can be generated using htpasswd or openssl
* Generated using openssl:

```
printf "user1:$(openssl passwd -apr1 123456)\n" >> ./userfile
```

* Password configuration example

```  
# user1, 123456
user1:$apr1$mI7SilJz$CWwYJyYKbhVDNl26sdUSh/
user2:{SHA}fEqNCco3Yq9h5ZUglD3CZJT4lBs=:user2, 123456
```

### Example

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
    Version": "20190101000000"
}
```
