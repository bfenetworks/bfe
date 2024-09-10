# mod_rewrite

## Introduction

mod_rewrite modifies the URI of HTTP request based on defined rules.

## Module Configuration

### Description

conf/mod_rewrite/mod_rewrite.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.DataPath | String<br>Path of rule configuration |

### Example

```ini
[Basic]
DataPath = mod_rewrite/rewrite.data
```

## Rule Configuration

### Description

conf/mod_rewrite/rewrite.data

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of config file |
| Config      | Struct<br>Rewrite rules for each product |
| Config{k}   | String<br>Product name |
| Config{v}   | Object<br>A ordered list of rewrite rules |
| Config{v}[] | Object<br>A rewrite rule |
| Config{v}[].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].Actions | Object<br>A ordered list of rewrite actions |
| Config{v}[].Actions[] | Object<br>A rewrite action |
| Config{v}[].Actions[].Cmd | Object<br>Name of rewrite action |
| Config{v}[].Actions[].Params | Object<br>Parameters of rewrite action |
| Config{v}[].Last | Integer<br>If true, stop to check the remaining rules |

### Actions

| Action                    | Description                              |
| ------------------------- | ---------------------------------------- |
| HOST_SET                  | Set host to specified value              |
| HOST_SET_FROM_PATH_PREFIX | Set host to specified path prefix        |
| HOST_SUFFIX_REPLACE       | Replace suffix of host                   |
| PATH_SET                  | Set path to specified value              |
| PATH_PREFIX_ADD           | Add prefix to original path               |
| PATH_PREFIX_TRIM          | Trim prefix from original path            |
| QUERY_ADD                 | Add query                                |
| QUERY_DEL                 | Delete query                             |
| QUERY_DEL_ALL_EXCEPT      | Del all queries except specified queries |
| QUERY_RENAME              | Rename query                             |
  
### Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_prefix_in(\"/rewrite\", false)",
                "Actions": [
                    {
                        "Cmd": "PATH_PREFIX_ADD",
                        "Params": [
                            "/bfe/"
                        ]
                    }
                ],
                "Last": true
            }
        ]
    }
}
```
  