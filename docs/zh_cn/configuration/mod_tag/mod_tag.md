# 简介 

根据自定义的条件，为请求设置Tag标识。

# 配置

- 模块配置文件

  conf/mod_tag/mod_tag.conf

  ```
  [Basic]
  DataPath = ../conf/mod_tag/tag_rule.data
  
  [Log]
  OpenDebug = false
  ```

- 规则配置文件

  conf/mod_tag/tag_rule.data

  | 配置项   | 类型   | 描述                                     |
  | ------- | ------ | --------------------------------------- |
  | Version | String | 配置文件版本                              |
  | Config  | Map    | key是产品线名称，value是产品线的规则列表     |

  产品线规则配置
  
  | 配置项          | 描述                                |
  | -------------- | ----------------------------------- | 
  | Cond           | 匹配条件                             |
  | Param.TagName  | Tag名称                             |
  | Param.TagValue | Tag值                               |
  | Last           | 如果值为true，命中规则后不在继续向下匹配 |
  
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
