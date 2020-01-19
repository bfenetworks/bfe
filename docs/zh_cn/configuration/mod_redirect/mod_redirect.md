# 简介

根据自定义的条件，对请求进行重定向。

# 配置

- 模块配置文件

  conf/mod_redirect/mod_redirect.conf

  ```
  [basic]
  DataPath = ../conf/mod_redirect/redirect.data
  ```

- 规则配置文件

  conf/mod_redirect/redirect.data

| 配置项  | 类型   | 描述                                                         |
| ------- | ------ | ------------------------------------------------------------ |
| Version | String | 配置文件版本                                                 |
| Config  | Struct | 基于产品线的重定向规则。每条规则包括： <br>- Cond: 描述匹配请求的条件<br>- Actions: 匹配成功后的动作<br>- Status: 响应HTTP状态码 |

| 动作           | 描述                                              |
| -------------- | ------------------------------------------------- |
| URL_SET        | 设置重定向URL为指定值                             |
| URL_FROM_QUERY | 设置重定向URL为指定请求Query值                    |
| URL_PREFIX_ADD | 设置重定向URL为原始URL增加指定前缀                |
| SCHEME_SET     | 设置重定向URL为原始URL并修改协议(支持HTTP和HTTPS) |

  ```
  {
      "Version": "20190101000000",
      "Config": {
          "example_product": [
              {
                  "Cond": "req_path_prefix_in(\"/redirect\", false)",
                  "Actions": [
                      {
                          "Cmd": "URL_SET",
                          "Params": ["https://example.org"]
                      }
                  ],
                  "Status": 301
              }
          ]
      }
  }
  ```

  
