# Redirect

## 场景说明

* 假设我们的web server已经升级为https，希望将http请求都重定向至https
  * 域名：example.org

在基础的[分流转发配置](分流转发.md)之上添加一些配置，就可以实现redirect，完整的配置详见[redirect](../../../example_conf/redirect)

* Bfe启用mod_redirect模块
  * 完整的bfe配置文件详见[bfe.conf](../../../example_conf/redirect/bfe.conf)

```
+++ Modules = mod_redirect  (bfe.conf中增加一行，启用mod_redirect)
```

* 配置redirect模块（[配置文件](../../../example_conf/redirect/mod_redirect/mod_redirect.conf)）
  * 配置redirect规则文件的存储路径

```
[basic]
DataPath = mod_redirect/redirect.data
```

* 配置redirect规则（[配置文件](../../../example_conf/redirect/mod_redirect/redirect.data)）
  * 域名为example.org的http请求重定向为https请求

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

* 现在，用curl验证下是否可以配置成功

curl -H "host: example.org" "http://127.1:8080/test"  将返回301，Location指向https://example.org/test
