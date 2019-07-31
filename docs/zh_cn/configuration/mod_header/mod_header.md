# 简介 

根据自定义条件，修改请求或响应的头部。

# 配置

- 模块配置文件

  conf/mod_header/mod_header.conf

  ```
  [basic]
  DataPath = ../conf/mod_header/header_rule.data
  ```

- 规则配置文件

  conf/mod_header/header_rule.data

  | 配置项  | 类型   | 描述                                                         |
  | ------- | ------ | ------------------------------------------------------------ |
  | Version | String | 配置文件版本                                                 |
  | Config  | Struct | 基于产品线的规则配置，每条规则包括： <br>- Cond: 描述匹配请求的条件<br>- Actions: 匹配成功后的动作<br>- Last: 当该项为true时，命中某条规则后，不再向后匹配 |

  | 动作           | 描述       |
  | -------------- | ---------- |
  | REQ_HEADER_SET | 设置请求头 |
  | REQ_HEADER_ADD | 添加请求头 |
  | RSP_HEADER_SET | 设置响应头 |
  | RSP_HEADER_ADD | 添加响应头 |
  | REQ_HEADER_DEL | 删除请求头 |
  | RSP_HEADER_DEL | 删除响应头 |
  | REQ_HEADER_MOD | 修改请求头 |
  | RSP_HEADER_MOD | 修改响应头 |

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

  
