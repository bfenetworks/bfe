# 分流转发

## 场景说明

* 假设我们有一个http server对外提供服务，并且有2个服务实例；1个负责处理静态文件请求，另外1个负责处理动态请求
  * 域名：example.org
  * 以/static开头的请求都转发至静态文件服务实例；地址：10.0.0.1:8001
  * 其他的请求都转发至动态服务实例；地址：10.0.0.1:8002

## 配置说明

在[样例配置](../../../conf/)上稍做修改，就可以实现上述转发功能

* Step 1.在 conf/bfe.conf配置转发功能使用的配置文件路径

```ini
hostRuleConf = server_data_conf/host_rule.data      #域名规则配置文件
routeRuleConf = server_data_conf/route_rule.data    #分流规则配置文件
clusterConf = server_data_conf/cluster_conf.data    #集群配置文件

clusterTableConf = cluster_conf/cluster_table.data  #集群实例列表配置文件
gslbConf = cluster_conf/gslb.data                   #子集群负载均衡配置文件
```

* Step 2. 配置域名规则 (conf/server_data_conf/host_rule.data)

```json
{
    "Version": "init version",
    "DefaultProduct": null,
    "Hosts": {
        "exampleTag":[
            "example.org" // 域名example.org=>域名标签exampleTag
        ]
    },
    "HostTags": {
        "example_product":[
            "exampleTag" // 域名标签exampleTag=>产品线名称example_product
        ]
    }
}
```

* Step 3. 配置集群的基础信息 (conf/server_data_conf/cluster_conf.data)
配置集群cluster_demo_static和cluster_demo_dynamic健康检查的参数，其他均使用默认值

```json
{
    "Version": "init version",
    "Config": {
        "cluster_demo_static": {                    // 集群cluster_demo_static的配置
            "CheckConf": {                          // 健康检查配置
                "Schem": "http",
                "Uri": "/health_check",
                "Host": "example.org",
                "StatusCode": 200
            }
        },
        "cluster_demo_dynamic": {                   // 集群cluster_demo_dynamic的配置
            "CheckConf": {                          // 健康检查配置
                "Schem": "http",
                "Uri": "/health_check",
                "Host": "example.org",
                "StatusCode": 200
            }
        }
    }
}
```

* Step 4. 配置集群下实例信息 (conf/cluster_conf/cluster_table.data)

```json
{
    "Version": "init version",
    "Config": {
        "cluster_demo_static": {         // 集群 => 子集群 => 实例列表
            "demo_static.all": [{        // 子集群demo_static.all
                "Addr": "10.0.0.1",      // 实例地址:10.0.0.1
                "Name": "static.A",      // 实例名:static.A
                "Port": 8001,            // 实例端口:8001
                "Weight": 1              // 实例权重:1
            }]
        },
        "cluster_demo_dynamic": {
            "demo_dynamic.all": [{
                "Addr": "10.0.0.1",
                "Name": "dynamic.A",
                "Port": 8002,
                "Weight": 1
            }]
        }
    }
}
```

* Step 5. 配置子集群内负载均衡 (conf/cluster_conf/gslb.data)

```json
{
    "Hostname": "",
    "Ts": "0",
    "Clusters": {
        "cluster_demo_static": {       // 集群 => 子集群权重
            "GSLB_BLACKHOLE": 0,       // 黑洞的分流权重为0，表示不丢弃流量
            "demo_static.all": 100     // 权重为100，表示全部分流到demo_static.all
        },
        "cluster_demo_dynamic": {
            "GSLB_BLACKHOLE": 0,
            "demo_dynamic.all": 100
        }
    }
}
```

* Step 6. 配置分流规则 (conf/server_data_conf/route_rule.data)
  * 将/static开头的流量转发到cluster_demo_static集群
  * 其余流量转发到cluster_demo_dynamic集群

```json
{
    "Version": "init version",
    "ProductRule": {
        "example_product": [    // 产品线 => 分流规则
            {
                // 以/static开头的path分流到cluster_demo_static集群
                "Cond": "req_path_prefix_in(\"/static\", false)",  
                "ClusterName": "cluster_demo_static"
            },
            {
                // 其他流量分流到cluster_demo_dynamic集群
                "Cond": "default_t()",
                "ClusterName": "cluster_demo_dynamic"
            }
        ]
    }
}
```

* Step 7. 验证配置规则

```bash
curl -H "host: example.org" "http://127.1:8080/static/test.html"  
# 将请求转发至10.0.0.1:8001

curl -H "host: example.org" "http://127.1:8080/api/test" 
# 将请求转发至10.0.0.1:8002
```
