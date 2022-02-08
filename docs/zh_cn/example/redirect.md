# 重定向

## 场景说明

* 假设我们的web server已经升级为https，希望将所有的http请求重定向至https
  * 域名：example.org

## 配置说明

在样例配置(conf/)上添加一些新的配置，就可以实现上述重定向行为

* Step 1. bfe启用mod_redirect模块 (conf/bfe.conf)

```ini
Modules = mod_redirect  #启用mod_redirect
```

* Step 2. 配置redirect规则文件的存储路径 (conf/mod_redirect/mod_redirect.conf)
  
```ini
[Basic]
DataPath = mod_redirect/redirect.data
```
  
* Step 3. 修改redirect规则文件 (conf/mod_redirect/redirect.data)
将域名为example.org的所有http请求重定向为https请求
  
```json
{
    "Version": "init version",
    "Config": {
        "example_product": [{
            "Cond": "!req_proto_secure() && req_host_in(\"example.org\")",
            "Actions": [{
                "Cmd": "SCHEME_SET",
                "Params": [
                    "https"
                ]
            }],
            "Status": 301
        }]
    }
}
```
  
* Step 4. 验证置规则

```bash
curl -H "host: example.org" "http://127.1:8080/test"
```

将返回301响应，响应Location头部为https://example.org/test
