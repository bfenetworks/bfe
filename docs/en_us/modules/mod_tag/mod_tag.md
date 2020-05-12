# mod_tag

## Introduction 

mod_tag sets tags for requests based on defined rules.

## Module Configuration

### Description
conf/mod_tag/mod_tag.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.DataPath | String<br>path of rule configuraiton |
| Log.OpenDebug | Boolean<br>debug flag of module |

### Example
```
[Basic]
DataPath = mod_tag/tag_rule.data

[Log]
OpenDebug = true
```

## Rule Configuration

### Description
conf/mod_tag/tag_rule.data

| Config Item | Description                                             |
| ----------- | ------------------------------------------------------- |
| Version     | String<br>Verson of the config file |
| Config      | Object<br>Tag rules for each product |
| Config{k}   | String<br>Product name |
| Config{v}   | Object<br>A list of tag rules |
| Config{v}[] | Object<br>A tag rule |
| Config{v}[].Cond           | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].Param.TagName  | String<br>tag name                                   |
| Config{v}[].Param.TagValue | String<br>tag value                                  |
| Config{v}[].Last           | Boolean<br>if true, stop to check the remaining rules |

### Example

```json
{
  "Version": "20200218210000",
  "Config": {
    "example_product": [
      {
        "Cond": "req_host_in(\"example.org\")",
        "Param": {
          "TagName": "tag",
          "TagValue": "bfe"
        },
        "Last": false
      }
    ]
  }
}
```
