# 简介 

根据自定义的条件，修改请求的URI。

# 配置

- 模块配置文件

  conf/mod_rewrite/mod_rewrite.conf

  ```
  [basic]
  DataPath = ../conf/mod_rewrite/rewrite.data
  ```

- 规则配置文件

  conf/mod_rewrite/rewrite.data

| 配置项  | 类型   | 描述                                                         |
| ------- | ------ | ------------------------------------------------------------ |
| Version | String | 配置文件版本                                                 |
| Config  | Struct | 基于产品线的重写规则，每条规则包括：<br>- Cond: 描述匹配请求的条件<br/>- Actions: 匹配成功后的动作<br/>- Last: 当该项为true时，命中某条规则后，不再向后匹配 |

| 动作                      | `描述`                             |
| ------------------------- | ---------------------------------- |
| HOST_SET_FROM_PATH_PREFIX | 根据path前缀设置host               |
| HOST_SET                  | 设置host                           |
| PATH_SET                  | 设置path                           |
| PATH_PREFIX_ADD           | 增加path前缀                       |
| PATH_PREFIX_TRIM          | 删除path前缀                       |
| QUERY_ADD                 | 增加query                          |
| QUERY_DEL                 | 删除query                          |
| QUERY_RENAME              | 重命名query                        |
| QUERY_DEL_ALL_EXCEPT      | 删除除指定key外的所有query         |

  ```
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
