# Routing

## Scenario

* Suppose we have a http server and it has two service instances, one is responsible for static file requests and the other one is responsible for dynamic requests
  * Hostname：example.org
  * all requests whose url path starts with /static forward to static file service instance with address 1.1.1.1:8001
  * other requests forward to dynamic service instance with address 1.1.1.1:8002

We can configure our routing requirements in this way. See the complete configuration [route](../../../example_conf/route) for details.

* Step 1. Map domain (example.org) to product (example_product)
  * configure [host_rule.data](../../../example_conf/route/server_data_conf/host_rule.data) as below

```
{
    "Version": "init version",
    "DefaultProduct": null,
    "Hosts": {
        "exampleTag":[
            "example.org" // domain(example.org)=>domainTag(exampleTag)
        ]
    },
    "HostTags": {
        "example_product":[
            "exampleTag" // domainTag(exampleTag)=>productName(example_product)
        ]
    }
}
```

* Step 2. Configure cluster parameters such as timeout, retransmission, health check, etc.
  * configure [cluster_conf.data](../../../example_conf/route/server_data_conf/cluster_conf.data) as below

```
{
    "Version": "init version",
    "Config": {
        "cluster_demo_static": {                // config for cluster_demo_static
            "BackendConf": {
                "TimeoutConnSrv": 2000,         // Timeout for connect backend:2s
                "TimeoutResponseHeader": 50000, // Timeout for read response header: 50s
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0
            },
            "CheckConf": {                      // health check config
                "Schem": "http",
                "Uri": "/health_check",
                "Host": "example.org",
                "StatusCode": 200,
                "FailNum": 10,
                "CheckInterval": 1000
            },
            "GslbBasic": {                      // GSLB config
                "CrossRetry": 0,
                "RetryMax": 2,
                "HashConf": {
                    "HashStrategy": 0,
                    "HashHeader": "Cookie:USERID",
                    "SessionSticky": false
                }
            },
            "ClusterBasic": {
                // Timeout for read client body:30s
                "TimeoutReadClient": 30000,
                // Timeout for write response:60s
                "TimeoutWriteClient": 60000, 
                // Timeout for read client again:30s
                "TimeoutReadClientAgain": 30000,
                // Write buffer size for request:512B
                "ReqWriteBufferSize": 512,
                // Flush request interval, 0 means no periodically flush
                "ReqFlushInterval": 0,
                // Flush response interval, -1 means no buffer and no periodically flush
                "ResFlushInterval": -1,
                "CancelOnClientClose": false
            }
        },
        "cluster_demo_dynamic": {                   // config for cluster_demo_dynamic
            "BackendConf": {
                "TimeoutConnSrv": 2000,
                "TimeoutResponseHeader": 50000,
                "MaxIdleConnsPerHost": 0,
                "RetryLevel": 0
            },
            "CheckConf": {
                "Schem": "http",
                "Uri": "/health_check",
                "Host": "example.org",
                "StatusCode": 200,
                "FailNum": 10,
                "CheckInterval": 1000
            },
            "GslbBasic": {
                "CrossRetry": 0,
                "RetryMax": 2,
                "HashConf": {
                    "HashStrategy": 0,
                    "HashHeader": "Cookie:USERID",
                    "SessionSticky": false
                }
            },
            "ClusterBasic": {
                "TimeoutReadClient": 30000,
                "TimeoutWriteClient": 60000,
                "TimeoutReadClientAgain": 30000,   
                "ReqWriteBufferSize": 512,
                "ReqFlushInterval": 0,
                "ResFlushInterval": -1,
                "CancelOnClientClose": false
            }
        }
    }
}
```

* Step 3. Configure subcluster instances mounted on clusters
  * create subcluster demo_static.all for cluster_demo_static
  * create subcluster demo_dynamic.all for  cluster_demo_dynamic
  * mount static file service instance(1.1.1.1:8001) on subcluster demo_static.all
  * mount dynamic service instance(1.1.1.1:8002) on subcluster demo_dynamic.all
  * configure [cluster_table.data](../../../example_conf/route/cluster_conf/cluster_table.data) as below

```
{
    "Version": "init version",
    "Config": {
        "cluster_demo_static": {         // cluster => sub-cluster => instances
            "demo_static.all": [{        // subcluster name
                "Addr": "1.1.1.1",       // address:1.1.1.1
                "Name": "static.A",      // instanceName
                "Port": 8001,            // port:8001
                "Weight": 1              // weight:1
            }]
        },
        "cluster_demo_dynamic": {
            "demo_dynamic.all": [{
                "Addr": "1.1.1.1",
                "Name": "dynamic.A",
                "Port": 8002,
                "Weight": 1
            }]
        }
    }
}
```

* Step 4. Configure load balance between sub-clusters
  * all requests for cluster_demo_static forward to demo_static.all
  * all requests for cluster_demo_dynamic forward to demo_dynamic.all
  * configure [gslb.data](../../../example_conf/route/cluster_conf/gslb.data) as below

```
{
    "Hostname": "",
    "Ts": "0",
    "Clusters": {
        "cluster_demo_static": {       // cluster => weight for each sub-cluster
            // GSLB_BLACKHOLE means discard traffic，set to 0 means do not discard traffic
            "GSLB_BLACKHOLE": 0,  
            // weight set to 100 means demo_static.all will carry all traffic
            "demo_static.all": 100
        },
        "cluster_demo_dynamic": {
            "GSLB_BLACKHOLE": 0,
            "demo_dynamic.all": 100
        }
    }
}
```

* Step 5. Configure route rules
  * requests started with /static forward to cluster_demo_static
  * other requests forward to cluster_demo_dynamic
  * configure [route_rule.data](../../../example_conf/route/server_data_conf/route_rule.data) as below

```
{
    "Version": "init version",
    "ProductRule": {
        "example_product": [    // product => route rules
            {
                // Requests started with /static forward to cluster_demo_static
                "Cond": "req_path_prefix_in(\"/static\", false)",  
                "ClusterName": "cluster_demo_static"
            },
            {
                // Other requests forward to cluster_demo_dynamic
                "Cond": "default_t()",
                "ClusterName": "cluster_demo_dynamic"
            }
        ]
    }
}
```

Now, use curl to verify whether it can be forwarded successfully.

curl -H "host: example.org" "http://127.1:8080/static/test.html"  will forward to 1.1.1.1:8001

curl -H "host: example.org" "http://1271:8080/api/test"  will forward to 1.1.1.1:8002