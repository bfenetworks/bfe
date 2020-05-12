# mod_userid

## Introduction 

mod_userid generates user id for client identification.

## Module Configuration

### Description
conf/mod_userid/mod_userid.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.DataPath | String<br>path of rule configuraiton |
| Log.OpenDebug | Boolean<br>debug flag of module |

### Example
```
[basic]
DataPath = mod_userid/userid_rule.data

[Log]
OpenDebug = true
```

## Rule Configuration

### Description
conf/mod_userid/userid_rule.data

| Config Item | Description                                             |
| ----------- | ------------------------------------------------------- |
| Version     | String<br>Verson of config file |
| Config | Object<br>rules for each product |
| Config{k} | String<br>Product name |
| Config{v} | Object<br>A list of rules |
| Config{v}[] | Object<br>A rule |
| Config{v}[].Cond          | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].Params.Name   | String<br>the cookie name        |
| Config{v}[].Params.Domain | String<br>the cookie domain      |
| Config{v}[].Params.Path   | String<br>the cookie path        |
| Config{v}[].Params.MaxAge | Integer<br>the cookie max age     |

### Example
```
{
    "Version": "2019-12-10184356",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_prefix_in(\"/abc\", true)",
                "Params": {
                     "Name": "bfe_userid_abc",
                     "Domain": "",
                     "Path": "/abc",
                     "MaxAge": 3153600
                 },
                 "Generator": "default"
            }, 
            {
                "Cond": "default_t()",
                "Params": {
                     "Name": "bfe_userid",
                     "Domain": "",
                     "Path": "/",
                     "MaxAge": 3153600
                 }
            }
        ]
    }
}
```
