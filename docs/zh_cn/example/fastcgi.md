# FCGI协议

## 场景说明

* 假设我们有一个http server对外提供服务，并且有2个服务实例；1个负责处理FastCGI协议请求，另外1个负责处理HTTP协议请求
  * 域名：example.org
  * 以/fcgi开头的请求都转发至FastCGI协议服务实例；地址：10.0.0.1:8001
  * 其他的请求都转发至HTTP协议服务实例；地址：10.0.0.1:8002

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
配置集群cluster_demo_fcgi和cluster_demo_http 后端配置的参数，其他均使用默认值

```json
{
    "Version": "init version",
    "Config": {
        "cluster_demo_http": {                   // 集群cluster_demo_http的配置
            "BackendConf": {
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0
            }
        },
        "cluster_demo_fcgi": {                    // 集群cluster_demo_fcgi的配置
            "BackendConf": {
                "Protocol": "fcgi",
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0,
                "FCGIConf": {
                    "Root": "/home/work",
                    "EnvVars": {
                        "VarKey": "VarVal"
                    }    
                }
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
        "cluster_demo_fcgi": {         // 集群 => 子集群 => 实例列表
            "demo_fcgi.all": [{        // 子集群demo_fcgi.all
                "Addr": "10.0.0.1",      // 实例地址:10.0.0.1
                "Name": "fcgi.A",      // 实例名:fcgi.A
                "Port": 8001,            // 实例端口:8001
                "Weight": 1              // 实例权重:1
            }]
        },
        "cluster_demo_http": {
            "demo_http.all": [{
                "Addr": "10.0.0.1",
                "Name": "http.A",
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
        "cluster_demo_fcgi": {       // 集群 => 子集群权重
            "GSLB_BLACKHOLE": 0,       // 黑洞的分流权重为0，表示不丢弃流量
            "demo_fcgi.all": 100     // 权重为100，表示全部分流到demo_fcgi.all
        },
        "cluster_demo_http": {
            "GSLB_BLACKHOLE": 0,
            "demo_http.all": 100
        }
    }
}
```

* Step 6. 配置分流规则 (conf/server_data_conf/route_rule.data)
  * 将/fcgi开头的流量转发到cluster_demo_fcgi集群
  * 其余流量转发到cluster_demo_http集群

```json
{
    "Version": "init version",
    "ProductRule": {
        "example_product": [    // 产品线 => 分流规则
            {
                // 以/fcgi开头的path分流到cluster_demo_fcgi集群
                "Cond": "req_path_prefix_in(\"/fcgi\", false)",  
                "ClusterName": "cluster_demo_fcgi"
            },
            {
                // 其他流量分流到cluster_demo_http集群
                "Cond": "default_t()",
                "ClusterName": "cluster_demo_http"
            }
        ]
    }
}
```

* Step 7. 验证配置规则

```bash
curl -H "host: example.org" "http://127.1:8080/fcgi/test"  
# 将请求转发至10.0.0.1:8001

curl -H "host: example.org" "http://127.1:8080/http/test" 
# 将请求转发至10.0.0.1:8002
```
