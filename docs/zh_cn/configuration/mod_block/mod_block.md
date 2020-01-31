# 简介 

基于预定义的规则，对连接或请求进行封禁。

# 配置

- 模块配置文件

  conf/mod_block/mod_block.conf

  ```
  [basic]
  # product rule config file path
  ProductRulePath = ../conf/mod_block/block_rules.data
  
  # global ip blacklist file path
  IPBlacklistPath = ../conf/mod_block/ip_blacklist.data
  ```

- 数据文件

  - 全局IP黑名单文件

    conf/mod_block/ip_blacklist.data

    ```
    192.168.1.253 192.168.1.254
    192.168.1.250
    ```

  - 封禁规则文件

    conf/mod_block/block_rules.data

| Config Item | 类型   |                                                              |
| ----------- | ------ | ------------------------------------------------------------ |
| Version     | String | 配置文件版本                                                 |
| Config      | Struct | 各产品线的封禁规则列表，每个规则包括：<br>- Cond: 描述匹配请求或连接的条件<br>- Action: 匹配成功后的动作 <br>- Name: 规则名称|

| 动作  | 描述     |
| ----- | -------- |
| CLOSE | 关闭连接 |

    ```
    {
      "Version": "20190101000000",
      "Config": {
          "example_product": [
              {
                "action": {
                      "cmd": "CLOSE",
                      "params": []
                  },
                  "name": "example rule",
                  "cond": "req_path_in(\"/limit\", false)"            
              }
          ]
      }
    }
    ```

  
