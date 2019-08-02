# Rewrite

## 场景说明

* 假设我们的服务升级后PATH发生了变化，已经发布出去的APP无法修改
* 希望老路径的请求可以自动修改PATH，而不用维护两套服务路径
  * 老PATH：/service
  * 新PATH：/v1/service

在基础的[分流转发配置](分流转发.md)之上启用rewrite模块并添加一些配置就可以实现这样的功能，完整的配置详见[rewrite](../../../example_conf/rewrite)

* Bfe启用mod_rewrite模块
  * 完整的bfe配置文件详见[bfe.conf](../../../example_conf/rewrite/bfe.conf)

```
+++ Modules = mod_rewrite  (bfe.conf中增加一行)
```

* 配置rewrite模块（[配置文件](../../../example_conf/rewrite/mod_rewrite/mod_rewrite.conf)）
  * 配置使用的rewrite规则文件的存储路径

```
[basic]
DataPath = mod_rewrite/rewrite.data
```

* 配置rewrite规则（[配置文件](../../../example_conf/rewrite/mod_rewrite/rewrite.data)）
  * 域名为example.org的所有请求均会添加/v1前缀后转发到后端集群

```json
{
    "Version": "init version",
    "Config": {
        "example_product": [{
            "Cond": "req_host_in(\"example.org\")",
            "Actions": [{
                "Cmd": "PATH_PREFIX_ADD",
                "Params": [
                    "/v1/"
                ]
            }],
            "Last": true
        }]
    }
}
```

* 现在，用curl验证下是否配置成功

curl -H "host: example.org" "http://127.1:8080/service", 后端集群cluster_demo_dynamic收到的请求PATH为"v1/service"
