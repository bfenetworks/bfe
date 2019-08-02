# Block

## 场景说明

* 假设我们的服务遭受了来自特定IP的攻击，或特定接口（如发放代金券）被恶意调用；希望可以简单粗暴的对流量进行封禁，如：
  * 封禁IP：攻击流量来自某些固定的IP（2.2.2.2）
  * 封禁PATH：攻击流量针对某些特定的PATH（/bonus）

在基础的[分流转发配置](分流转发.md)之上启用block模块并添加block规则就可以实现这样的功能，完整的配置详见[block](../../../example_conf/block)

* Bfe启用mod_block模块
  * 完整的bfe配置文件详见[bfe.conf](../../../example_conf/block/bfe.conf)

```
+++ Modules = mod_block  (bfe.conf中增加一行)
```

* 配置block模块（[配置文件](../../../example_conf/block/mod_block/mod_block.conf)）
  * 配置使用的block规则文件（包括全局IP黑名单和封禁规则）的存储路径

```
[basic]
# 封禁规则文件路径
ProductRulePath = mod_block/block_rules.data

# IP黑名单文件路径
IPBlacklistPath = mod_block/ip_blacklist.data
```

* 配置block规则
  * 通过IP黑名单封禁IP地址：2.2.2.2（[配置文件](../../../example_conf/block/mod_block/ip_blacklist.data)）
  
  ```
  2.2.2.2
  ```
    
  * 封禁PATH：/bonus（[配置文件](../../../example_conf/block/mod_block/block_rules.data)）
  
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
