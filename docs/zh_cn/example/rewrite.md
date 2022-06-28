# 重写

## 场景说明

* 假设我们的服务升级后，服务路径发生了变化（/service => /v1/service）
* 同时我们有一些已经发布出去的APP，如果修改请求路径，需要用户下载安装新版本
* 我们希望老版本的APP也可以直接请求新的服务，而不用同时维护两套服务

## 配置说明

在样例配置(conf/)上添加一些新的配置，就可以实现上述功能

* Step 1. bfe启用mod_rewrite模块（conf/bfe.conf)

```ini
Modules = mod_rewrite  #启用mod_rewrite
```

* Step 2. 配置rewrite规则文件的存储路径 (conf/mod_rewrite/mod_rewrite.conf)
  
```ini
[Basic]
DataPath = mod_rewrite/rewrite.data
```
  
* Step 3. 配置rewrite规则
  
路径前缀为/service的所有请求均会添加/v1前缀后转发给后端服务
  
```json
{
    "Version": "init version",
    "Config": {
        "example_product": [{
            "Cond": "req_path_prefix_in(\"/service\", false)",
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

* Step 4. 验证配置规则

```bash
curl -H "host: example.org" "http://127.1:8080/service"
```

对应后端服务集群cluster_demo_dynamic收到的请求PATH为"v1/service"
