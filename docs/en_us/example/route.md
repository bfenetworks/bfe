# Route

## Scenario

* Imagine we have an http server which has two instances. One is responsible for processing static file requests, and the other is responsible for dynamic requests.
  * Host：example.org
  * Requests that start with / static are forwarded to the static file service instance with address 10.0.0.1:8001
  * Other requests are forwarded to dynamic service instance with address 10.0.0.1:8002

## Configuration

Modify example configurations (conf/) as the following steps:

* Step 1. Config path of forward rules in conf/bfe.conf

```ini
hostRuleConf = server_data_conf/host_rule.data
routeRuleConf = server_data_conf/route_rule.data
clusterConf = server_data_conf/cluster_conf.data

clusterTableConf = cluster_conf/cluster_table.data
gslbConf = cluster_conf/gslb.data  
```

* Step 2. Config host rules (conf/server_data_conf/host_rule.data)

```json
{
    "Version": "init version",
    "DefaultProduct": null,
    "Hosts": {
        "exampleTag":[
            "example.org" // host name: example.org=>host tag: exampleTag
        ]
    },
    "HostTags": {
        "example_product":[
            "exampleTag" // host tag: exampleTag=>product name: example_product
        ]
    }
}
```

* Step 3. Config cluster configuration (conf/server_data_conf/cluster_conf.data)
Note: Set health check params and use default value for other params

```json
{
    "Version": "init version",
    "Config": {
        "cluster_demo_static": {
            "CheckConf": {                          // health check config
                "Schem": "http",
                "Uri": "/health_check",
                "Host": "example.org",
                "StatusCode": 200
            }
        },
        "cluster_demo_dynamic": {
            "CheckConf": {                          // health check config
                "Schem": "http",
                "Uri": "/health_check",
                "Host": "example.org",
                "StatusCode": 200
            }
        }
    }
}
```

* Step 4. Config instances of cluster (conf/cluster_conf/cluster_table.data)

```json
{
    "Version": "init version",
    "Config": {
        "cluster_demo_static": {         // cluster => sub_cluster => instance list
            "demo_static.all": [{        // subcluster: demo_static.all
                "Addr": "10.0.0.1",
                "Name": "static.A",
                "Port": 8001,
                "Weight": 1
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

* Step 5. Config gslb configuration (conf/cluster_conf/gslb.data)

```json
{
    "Hostname": "",
    "Ts": "0",
    "Clusters": {
        "cluster_demo_static": {   // cluster => weight of subcluster
            "GSLB_BLACKHOLE": 0,   // GSLB_BLACKHOLE == 0 means do not discard traffic
            "demo_static.all": 100 // weight 100 means all traffic routes to demo_static.all
        },
        "cluster_demo_dynamic": {
            "GSLB_BLACKHOLE": 0,
            "demo_dynamic.all": 100
        }
    }
}
```

* Step 6. Config route rules (conf/server_data_conf/route_rule.data)

```json
{
    "Version": "init version",
    "ProductRule": {
        "example_product": [    // product => route rules
            {
                "Cond": "req_path_prefix_in(\"/static\", false)",  
                "ClusterName": "cluster_demo_static"
            },
            {
                "Cond": "default_t()",
                "ClusterName": "cluster_demo_dynamic"
            }
        ]
    }
}
```

* Step 7. Verify configured rules

```bash
curl -H "host: example.org" "http://127.1:8080/static/test.html"  
# request will route to 10.0.0.1:8001

curl -H "host: example.org" "http://127.1:8080/api/test"  
# request will route to 10.0.0.1:8002
```
