# 黑名单封禁

## 场景说明

* 假设我们的服务遭受了来自特定IP的攻击，或特定接口（如发放代金券）被恶意调用；希望可以对指定流量进行封禁，如：
  * 封禁IP：攻击流量来自某些固定的IP（2.2.2.2）
  * 封禁PATH：攻击流量针对某些特定的PATH（/bonus）

## 配置说明

在样例配置(conf/)上添加一些新的配置，就可以实现上述封禁功能

* Step 1. bfe启用mod_block模块（conf/bfe.conf)

```ini
Modules = mod_block   #启用mod_block
```

* Step 2. 配置使用的block规则文件（包括全局IP黑名单和封禁规则）路径(mod_block/mod_block.conf)
  
```ini
[Basic]
# 封禁规则文件路径
ProductRulePath = mod_block/block_rules.data

# IP黑名单文件路径
IPBlocklistPath = mod_block/ip_blocklist.data
```
  
* Step 3. 配置全局IP黑名单 (mod_block/ip_blocklist.data)
  
 通过IP黑名单封禁IP地址：2.2.2.2
  
```
2.2.2.2
```
  
* Step 4. 配置封禁规则 (mod_block/block_rules.data)
  
```json
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

* Step 5. 验证封禁规则

```bash
curl -v -H "host: example.org" "http://127.1:8080/bonus"
```

连接将会被直接关闭
