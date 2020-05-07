# mod_header

## Introduction 

Modify header of HTTP request/response based on defined rules.

## Module Configuration

### Description
conf/mod_header/mod_header.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.DataPath | String<br>path of rule configuraiton |
| Log.OpenDebug | Boolean<br>debug flag of module |

### Example

```
[basic]
DataPath = mod_header/header_rule.data
```

## Rule Configuration

### Description
conf/mod_header/header_rule.data

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Verson of config file |
| Config      | Struct<br>Header rules for each product |
| Config{k}   | String<br>Product name |
| Config{v}   | Object<br>A ordered list of rules |
| Config{v}[] | Object<br>A rule |
| Config{v}[].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].Last | Boolean<br>If true, stop processing the next rule |
| Config{v}[].Actions | Object<br>A list of Actions |
| Config{v}[].Actions.Cmd | String<br>A Action |
| Config{v}[].Actions.Params | Object<br>A list of parameters for action |
| Config{v}[].Actions.Params[] | String<br>A parameter |

### Actions
| Action         | Description            |
| -------------- | ---------------------- |
| REQ_HEADER_SET | Set request header     |
| REQ_HEADER_ADD | Add request header     |
| RSP_HEADER_SET | Set response header    |
| RSP_HEADER_ADD | Add response header    |
| REQ_HEADER_DEL | Delete request header  |
| RSP_HEADER_DEL | Delete response header |
| REQ_HEADER_MOD | Modify request header  |
| RSP_HEADER_MOD | Modify response header |

### Example

```
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "cond": "req_path_prefix_in(\"/header\", false)",
                "actions": [
                    {
                        "cmd": "RSP_HEADER_SET",
                        "params": [
                            "X-Proxied-By",
                            "bfe"
                        ]
                    }
                ],
                "last": true
            }
        ]
    }
}
```
