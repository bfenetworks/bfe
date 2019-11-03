# 简介 

根据自定义条件，修改请求或响应的头部。

# 配置

## 模块配置文件

  conf/mod_header/mod_header.conf

  ```
  [basic]
  DataPath = ../conf/mod_header/header_rule.data
  ```

## 规则配置文件

  conf/mod_header/header_rule.data

  | 配置项  | 类型   | 描述                                                         |
  | ------- | ------ | ------------------------------------------------------------ |
  | Version | String | 配置文件版本                                                 |
  | Config  | Map&lt;String, Array&lt;HeaderRule&gt;&gt; | 各产品线的规则配置 |
  
- HeaderRule
  | 配置项  | 类型   | 描述                                                         |
  | ------- | ------ | ------------------------------------------------------------ |
  | Cond | String | 条件原语                                                 |
  | Actions  | Array&lt;Action&gt; | 执行动作列表 |

- Action
  | 配置项  | 类型   | 描述                                                         |
  | ------- | ------ | ------------------------------------------------------------ |
  | Cmd | String | 动作类型，详见下表                                                 |
  | Params  | Array&lt;String&gt; | 动作参数 |

  | 动作           | 描述       |
  | -------------- | ---------- |
  | REQ_HEADER_SET | 设置请求头 |
  | REQ_HEADER_ADD | 添加请求头 |
  | REQ_HEADER_MOD | 修改请求头 |
  | REQ_HEADER_DEL | 删除请求头 |
  | RSP_HEADER_SET | 设置响应头 |
  | RSP_HEADER_ADD | 添加响应头 |
  | RSP_HEADER_MOD | 修改响应头 |
  | RSP_HEADER_DEL | 删除响应头 |
  
# 示例

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
