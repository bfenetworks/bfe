# 重定向

## 场景说明

* 假设我们的web server已经升级为https，希望将所有的http请求重定向至https
  * 域名：example.org

在[样例配置](../../../conf/)上添加一些新的配置，就可以实现上述重定向行为

* 首先，bfe启用mod_redirect模块（[bfe.conf](../../../conf/bfe.conf)第51行）

```
Modules = mod_redirect  #启用mod_redirect
```

* 配置redirect模块
  
  * 配置redirect规则文件的存储路径（[mod_redirect/mod_redirect.conf](../../../conf/mod_redirect/mod_redirect.conf)）
  
  ```
  [basic]
  DataPath = mod_redirect/redirect.data
  ```
  
  * 修改redirect规则文件（[mod_redirect/redirect.data](../../../conf/mod_redirect/redirect.data)），将域名为example.org的所有http请求重定向为https请求
  
  ```
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
  
* 现在，用curl验证下是否配置成功

curl -H "host: example.org" "http://127.1:8080/test"  将返回301，Location指向https://example.org/test
