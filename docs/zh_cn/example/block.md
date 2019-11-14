# Block

## 场景说明

* 假设我们的服务遭受了来自特定IP的攻击，或特定接口（如发放代金券）被恶意调用；希望可以对指定流量进行封禁，如：
  * 封禁IP：攻击流量来自某些固定的IP（2.2.2.2）
  * 封禁PATH：攻击流量针对某些特定的PATH（/bonus）

在[样例配置](../../../conf/)上添加一些新的配置，就可以实现上述block功能

* 首先，bfe启用mod_block模块（[bfe.conf](../../../conf/bfe.conf)）

```
Modules = mod_block   #启用mod_block
```

* 配置block模块
  
  * 配置使用的block规则文件（包括全局IP黑名单和封禁规则）的存储路径（[mod_block/mod_block.conf](../../../conf/mod_block/mod_block.conf)）
  
  ```
  [basic]
  # 封禁规则文件路径
  ProductRulePath = mod_block/block_rules.data
  
  # IP黑名单文件路径
  IPBlacklistPath = mod_block/ip_blacklist.data
  ```
  
  * 配置block规则
  
    * 通过IP黑名单封禁IP地址：2.2.2.2（[mod_block/ip_blacklist.data](../../../conf/mod_block/ip_blacklist.data))
  
      ```
      2.2.2.2
      ```
  
    * 封禁PATH：/bonus（[mod_block/block_rules.data](../../../conf/mod_block/block_rules.data))
  
      ```
      {
          "Version": "init version",
          "Config": {
              "example_product": [{
                  "action": {
                      "cmd": "CLOSE",
                      "params": []
                  },
                  "name": "block bonus",
                  "cond": "req_path_in(\"/bonus\", false)"
              }]
          }
      }
      ```

* 现在，用curl验证下是否封禁成功

curl -v -H "host: example.org" "http://127.1:8080/bonus", 连接将会被直接关闭
