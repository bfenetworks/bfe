# BFE WAF Usage
本文演示如何使用bfe waf.

## 介绍
BFE 通过BWI支持统一的第三方WAF 接入。
关于BWI(BFE WAF Interface), 参考[BFE WAF Interface](https://github.com/bfenetworks/bwi)。
关于BFE Mock WAF Server，参考[BFE Mock WAF Server](https://github.com/bfenetworks/bfe-mock-waf)。
本文使用BFE Mock WAF Server演示BFE WAF模块的使用。

## 前置准备
本文会使用默认的bfe中的配置。
包含：
- host：example.org
- product: example_product 
- cluster: cluster_example
- subcluster: example.bfe.bj
- RS: 127.0.0.1:8181

### 启动BFE RS
这里使用如下构造的简化http server
#python3  simple_http_server.py 8181

```
# cat simple_http_server.py
import http.server
import socketserver
import sys

port = int(sys.argv[1])

class MyHttpRequestHandler(http.server.SimpleHTTPRequestHandler):
    def do_POST(self):
        return self.do_GET()
with socketserver.TCPServer(("", port), MyHttpRequestHandler) as httpd:
    print("Http Server Serving at port", port)
    httpd.serve_forever()
```


### 启动WAF Server
这里我们使用BFE Mock WAF Server，参考[BFE Mock WAF Server](https://github.com/bfenetworks/bfe-mock-waf)。
BFE默认集成了BFE Mock WAF Server。

切换到BFE Mock WAF Server的工作路径
#go run waf_server_demo.go
WAF HTTP server listening port:8899

## BFE配置修改

切换到BFE的工作路径(bin目录)。

### 打开mod_unified_waf 模块
确认模块mod_unified_waf打开了
```
#cat ../conf/bfe.conf
...
Modules = mod_unified_waf
...
```

### 修改使用的WAF产品

#### 把WafProductName改为BFEMockWaf
```
#cat ../conf/mod_unified_waf/mod_unified_waf.conf

[Basic]
#candidates: None, BFEMockWaf
WafProductName = BFEMockWaf
```

#### 确认mod_unified_waf参数

```
#cat ../conf/mod_unified_waf/mod_unified_waf.data
{
	"Version": "2025-06-23 12:00:10",
	"Config": {
        "WafClient": {
            "ConnectTimeout": 30,
            "Concurrency": 2000,
            "MaxWaitCount": 400
        },
        "WafDetect": {
            "RetryMax": 2,
            "ReqTimeout": 40
        },
        "HealthChecker": {
            "UnavailableFailedThres": 20,
            "HealthCheckInterval": 10000
        }
    }
}
```


#### 修改WAF RS 实例
```
#cat ../conf/mod_unified_waf/alb_waf_instances.data
{
        "Version": "2023-01-19 12:00:10",
        "Config": {
                "WafCluster": [
                        {"IpAddr": "127.0.0.1", "Port": 8899, "HealthCheckPort": 8899}
                ]

        }
}
```

#### 修改产品线检测参数
```
#cat ../conf/mod_unified_waf/product_param.data
{
        "Version": "2023-01-19 12:00:10",
        "Config": {
          "example_product": {
            "SendBody": true,
            "SendBodySize": 1024
          }

    }
}
```
注：这里最多只检测http req body的前面1024个字节。


## 启动BFE
#./bfe -d -c ../conf -l ../log

注：BFE 运行在 172.18.55.230 的机器上。下面会用到这个IP地址。

## 客户端访问

## curl访问
使用http GET
#curl -v -H "HOST:example.org" http://172.18.55.230:8080

使用http POST
#curl  -v  -X POST -H "HOST:example.org"   http://172.18.55.230:8080 -d @waf-body1023.data
注: waf-body1023.data是一个1023个字节的数据文件


