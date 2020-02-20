# Introduction 

Set tags for request based on defined rules.

# Configuration

- Module config file

  conf/mod_tag/mod_tag.conf

  ```
  [Basic]
  DataPath = mod_tag/tag_rule.data

  [Log]
  OpenDebug = true
  ```

- Rule config file

  conf/mod_tag/tag_rule.data

  | Config Item | Type   | Description                                             |
  | ----------- | ------ | ------------------------------------------------------- |
  | Version     | String | Verson of the config file                                   |
  | Products    | Map    | key is product name,value is the rule list of product   |
  
  Product rule config:

  | Config Item    | Description                                |
  | -------------- | ------------------------------------------ |
  | Cond           | "condition" expression                     |
  | Param.TagName  | tag name                                   |
  | Param.TagValue | tag value                                  |
  | Last           | if true, stop to check the remaining rules |

  ```
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

  

