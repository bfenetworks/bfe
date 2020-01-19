# 简介 

基于配置信任IP列表，检查并标识访问用户真实IP是否属于信任IP。

# 配置

- 模块配置文件

  conf/mod_trust_clientip/mod_trust_clientip.conf

  ```
  [basic]
  DataPath = ../conf/mod_trust_clientip/trust_client_ip.data
  ```

- 字典数据文件

  conf/mod_trust_clientip/trust_client_ip.data

| 配置项  | 类型   | 描述                                                                          |
| ------- | ------ | ----------------------------------------------------------------------------- |
| Version | String | 配置文件版本                                                                  |
| Config  | Struct | 记录信任的IP列表。包含多个Name/Value对，Name是标签，Value是IP段列表           |

  ```
  {
      "Version": "20190101000000",
      "Config": {
          "inner-idc": [
              {
                  "Begin": "10.0.0.0",
                  "End": "10.255.255.255"
              }
          ]
      }
  }
  ```

