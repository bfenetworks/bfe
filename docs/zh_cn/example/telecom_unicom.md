# 电信-联通CDN前端配置

## 场景说明
* 假设源站IP：1.1.1.1。有两个CDN节点：电信：2.2.2.2，联通：3.3.3.3
* 源站WEB服务器为Nginx，CDN采用varnish，前端使用开源bfe 0.4.0
* 做CDN加速的域名为：www.test1.com www.test2.com
* 本例子bfe配置文件在/usr/local/baidu_bfe/conf

* 源站Nginx 配置如下：
test1.com.conf
```
server {
        listen 80;
        server_name test1.com;
        rewrite ^(.*) https://www.test1.com$1 permanent; 
        # http://test1.com 301 跳转到 https://www.test1.com
        root /var/www/html/test1.com;

        location / {
            index  index.html index.htm index.php;
        }
}
```
www.test1.com.conf
```
server {
        listen 80;
        server_name www.test1.com;
        rewrite ^(.*) https://www.test1.com$1 permanent;
        # http://www.test1.com 301 跳转到 https://www.test1.com
        root /var/www/html/test1.com;

        location / {
            index  index.html index.htm index.php;
        }
}
```
test1.com_ssl.conf
```
server {
    listen       443 ssl http2;
    server_name  test1.com;
    rewrite ^(.*) https://www.test1.com$1 permanent;
    # https://test1.com 301 跳转到 https://www.test1.com 
    root /var/www/html/test1.com;

    location / {
            index  index.html index.htm index.php;
    }

    location ~ \.php(/|$) {
            include fastcgi_params;
            fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
            fastcgi_pass 127.0.0.1:9001;
    }

    location ~ /\.svn|/\.git {
            deny all;
            internal;
    }

    ssl_certificate /etc/nginx/ssl/www.test1.com.crt;
    ssl_certificate_key /etc/nginx/ssl/www.test1.com.key;
    ssl_session_cache shared:SSL:20m;
    ssl_session_timeout  60m;
    ssl_ciphers  ALL:!ADH:!EXPORT56:RC4+RSA:+HIGH:+MEDIUM:+LOW:+SSLv2:+EXP;
    ssl_prefer_server_ciphers on;
    ssl_protocols TLSv1 TLSv1.1 TLSv1.2;
    add_header Strict-Transport-Security "max-age=31536000" always;
}
```
www.test1.com_ssl.conf
```
server {
    listen       443 ssl http2;
    server_name  www.test1.com;
    
    root /var/www/html/test1.com;

    location / {
            index  index.html index.htm index.php;
    }

    location ~ \.php(/|$) {
            include fastcgi_params;
            fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
            fastcgi_pass 127.0.0.1:9001;
    }

    location ~ /\.svn|/\.git {
            deny all;
            internal;
    }

    ssl_certificate /etc/nginx/ssl/www.test1.com.crt;
    ssl_certificate_key /etc/nginx/ssl/www.test1.com.key;
    ssl_session_cache shared:SSL:20m;
    ssl_session_timeout  60m;
    ssl_ciphers  ALL:!ADH:!EXPORT56:RC4+RSA:+HIGH:+MEDIUM:+LOW:+SSLv2:+EXP;
    ssl_prefer_server_ciphers on;
    ssl_protocols TLSv1 TLSv1.1 TLSv1.2;
    add_header Strict-Transport-Security "max-age=31536000" always;
}
```
## 再配置供电信和联通前端varnish访问的8080端口：
www.test1.com_8080.conf
```
server {
        listen 8080;
        server_name www.test1.com;
        root /var/www/html/test1.com;

        location / {
            index  index.html index.htm index.php;
        }

        location ~ \.php(/|$) {
                include fastcgi_params;
                fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
                fastcgi_pass 127.0.0.1:9001;
        }

        location ~ /\.svn|/\.git {
            deny all;
            internal;
        }
}
```
## test2.com nginx配置和test1.com配置类似。

* 电信CDN varnish节点配置，联通CDN varnish节点配置类似。

/etc/varnish/varnish.params
```
RELOAD_VCL=1
VARNISH_VCL_CONF=/etc/varnish/default.vcl
VARNISH_LISTEN_ADDRESS=127.0.0.1
VARNISH_LISTEN_PORT=8080
# varnish 监听本地127.0.0.1 8080端口
VARNISH_ADMIN_LISTEN_PORT=6082
VARNISH_SECRET_FILE=/etc/varnish/secret
VARNISH_STORAGE="malloc,256M"
VARNISH_USER=varnish
VARNISH_GROUP=varnish 
```
/etc/varnish/default.vcl
```
vcl 4.0;
backend default {
    .host = "1.1.1.1";
    .port = "8080";
}
backend web1 {
    .host = "test1.com";
    .port = "8080";
}
backend web2 {
    .host = "test2.com";	
    .port = "8080";
}
acl purge {
 "127.0.0.1";
  "localhost";
}
sub vcl_recv {
    if (req.http.host ~ "^www.test1.com") {
           set req.backend_hint = web1;
    }
    elsif (req.http.host ~ "^test1.com") {
           set req.http.host = "www.test1.com";
           set req.backend_hint = web1;  
    }
    elsif (req.http.host ~ "^www.test2.com") {
           set req.backend_hint = web2;
    } 
    elsif (req.http.host ~ "^test2.com") {
           set req.http.host = "www.test2.com";
           set req.backend_hint = web2;
    } 
    else {
          return(synth(404,"tot's cache"));
    }
    if (req.restarts == 0) {
       if (req.http.X-Forwarded-For) {
           set req.http.X-Forwarded-For = req.http.X-Forwarded-For + ", " + client.ip;
       } else {
                set req.http.X-Forwarded-For = client.ip;
       }
    }
    if (req.method == "PURGE") {
       if (!client.ip ~ purge) {
           return (synth(405, "This IP is not allowed to send PURGE requests."));
       }
       return (purge);
    }
    if (req.method != "GET" &&
        req.method != "HEAD" &&
        req.method != "PUT" &&
        req.method != "POST" &&
        req.method != "TRACE" &&
        req.method != "OPTIONS" &&
        req.method != "PATCH" &&
        req.method != "DELETE") {
        /* Non-RFC2616 or CONNECT which is weird. */
        return (pipe);
    }
    if (req.method != "GET" && req.method != "HEAD") {
        return (pass);
    } 
    return (hash);
}
sub vcl_pass {
    return (fetch);
}
sub vcl_hash {
      hash_data(req.url);
      if (req.http.host) {
           hash_data(req.http.host);
      } else {
           hash_data(server.ip);
      }
    return (lookup);
}

sub vcl_backend_response {
    if (beresp.status == 499 || beresp.status == 404 || beresp.status == 502) {
       set beresp.uncacheable = true;
    }
    if (bereq.url ~ "\.(php|jsp)(\?|$)") {
       set beresp.uncacheable = true;
    } else {
    if (bereq.url ~ "\.html(\?|$)") {
       set beresp.ttl = 1h;
       unset beresp.http.Set-Cookie;
    } else {
       set beresp.ttl = 2h;
       unset beresp.http.Set-Cookie;
       }
    }
    set beresp.grace = 6h;
    return (deliver);
}

sub vcl_deliver {
    if (obj.hits > 0 ) {
       set resp.http.X-Cache = "HIT from tot's cache";
    }
    else {
       set resp.http.X-Cache ="MISS from tot's cache";
    }
    unset resp.http.X-Powered-By;
    unset resp.http.Server;
    unset resp.http.X-Drupal-Cache;
    unset resp.http.Via;
    unset resp.http.Link;
    unset resp.http.X-Varnish;
    unset resp.http.Age;
    return(deliver);
}
```
* 电信前端bfe 0.4.0配置，联通前端 bfe 0.4.0配置类似。

/usr/local/baidu_bfe/conf/mod_redirect/redirect.data
```
{
    "Version": "init version",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_prefix_in(\"/redirect\", false)",
                "Actions": [
                    {
                        "Cmd": "URL_SET",
                        "Params": ["https://example.org"]
                    }
                ],
                "Status": 301
            }
        ],
        "web1_product": [
            {
                "Cond": "req_host_in(\"test1.com\")",
                "Actions": [
                    {
                        "Cmd": "URL_SET",
                        "Params": ["https://www.test1.com"]
                    }
                ],
                "Status": 301
            } // http://test1.com和 https://test1.com跳转到 https://www.test1.com
        ],
        "web2_product": [
            {
                "Cond": "!req_proto_secure() && req_host_in(\"www.test1.com\")",
                "Actions": [
                    {
                        "Cmd": "URL_SET",
                        "Params": ["https://www.test1.com"]
                    }
                ],
                "Status": 301
            } // http://www.test1.com跳转到 https://www.test1.com
        ],
        "web3_product": [
            {
                "Cond": "req_host_in(\"test2.com\")",
                "Actions": [
                    {
                        "Cmd": "URL_SET",
                        "Params": ["https://www.test2.com"]
                    }
                ],
                "Status": 301
            } // http://test2.com和 https://test2.com跳转到 https://www.test2.com
        ],
        "web4_product": [
            {
                "Cond": "!req_proto_secure() && req_host_in(\"www.test2.com\")",
                "Actions": [
                    {
                        "Cmd": "URL_SET",
                        "Params": ["https://www.test2.com"]
                    }
                ],
                "Status": 301
            } // http://www.test2.com跳转到 https://www.test2.com
        ]
    }
}
```
/usr/local/baidu_bfe/conf/server_data_conf/host_rule.data
```
{
    "Version": "init version",
    "DefaultProduct": null,
    "Hosts": {
        "exampleTag":[
            "example.org"
        ],
        "web1Tag":[
            "test1.com"
        ],
        "web2Tag":[
            "www.test1.com"
        ],
        "web3Tag":[
            "test2.com"
        ],
        "web4Tag":[
            "www.test2.com"
        ]
    },
    "HostTags": {
        "example_product":[
            "exampleTag"
        ],
        "web1_product":[
            "web1Tag"
        ],
        "web2_product":[
            "web2Tag"
        ],
        "web3_product":[
            "web3Tag"
        ],
        "web4_product":[
            "web4Tag"
        ]
    }
}
```
/usr/local/baidu_bfe/conf/server_data_conf/route_rule.data
```
{
    "Version": "init version",
    "ProductRule": {
        "example_product": [
            {
                "Cond": "req_host_in(\"example.org\")",
                "ClusterName": "cluster_example"
            },
            {
                "Cond": "default_t()",
                "ClusterName": "cluster_example"
            }
        ],
        "web1_product": [
            {
                "Cond": "req_host_in(\"test1.com\")",
                "ClusterName": "cluster_web1"
            },
            {
                "Cond": "default_t()",
                "ClusterName": "cluster_web1"
            }
        ],
        "web2_product": [
            {
                "Cond": "req_host_in(\"www.test1.com\")",
                "ClusterName": "cluster_web1"
            },
            {
                "Cond": "default_t()",
                "ClusterName": "cluster_web1"
            }
        ],
        "web3_product": [
            {
                "Cond": "req_host_in(\"test2.com\")",
                "ClusterName": "cluster_web1"
            },
            {
                "Cond": "default_t()",
                "ClusterName": "cluster_web1"
            }
        ],
        "web4_product": [
            {
                "Cond": "req_host_in(\"www.test2.com\")",
                "ClusterName": "cluster_web1"
            },
            {
                "Cond": "default_t()",
                "ClusterName": "cluster_web1"
            }
        ]
    }
}
```
/usr/local/baidu_bfe/conf/cluster_conf/cluster_table.data
```
{
    "Config": {
        "cluster_example": {
            "example.bfe.bj": [
                {
                    "Addr": "10.199.189.26",
                    "Name": "example_hostname",
                    "Port": 10257,
                    "Weight": 10
                }
            ]
        },
        "cluster_web1": {
            "varnish.bfe.shenzhen": [
                {
                    "Addr": "127.0.0.1",
                    "Name": "varnish_hostname",
                    "Port": 8080,
                    "Weight": 10
                }
            ]
        }  //代理前端 varnish 本地127.0.0.1 8080 端口
    }, 
    "Version": "init version"
}
```
/usr/local/baidu_bfe/conf/cluster_conf/gslb.data
```
{
    "Clusters": {
        "cluster_example": {
            "GSLB_BLACKHOLE": 0,
            "example.bfe.bj": 100
        },
        "cluster_web1": {
            "GSLB_BLACKHOLE": 0,
            "varnish.bfe.shenzhen": 100
        }
    },
    "Hostname": "",
    "Ts": "0"
}
```
/usr/local/baidu_bfe/conf/tls_conf/server_cert_conf.data
```
{
    "Version": "init version",
    "Config": {
        "Default": "example.org",
        "CertConf": {
            "example.org": {
                "ServerCertFile": "../conf/tls_conf/certs/server.crt",
                "ServerKeyFile" : "../conf/tls_conf/certs/server.key"
            },
            "test1.com": {
                "ServerCertFile": "../conf/tls_conf/certs/www.test1.com.crt",
                "ServerKeyFile" : "../conf/tls_conf/certs/www.test1.com.key"
            },
            "www.test1.com": {
                "ServerCertFile": "../conf/tls_conf/certs/www.test1.com.crt",
                "ServerKeyFile" : "../conf/tls_conf/certs/www.test1.com.key"
            },
            "test2.com": {
                "ServerCertFile": "../conf/tls_conf/certs/www.test2.com.crt",
                "ServerKeyFile" : "../conf/tls_conf/certs/www.test2.com.key"
            },
            "www.test2.com": {
                "ServerCertFile": "../conf/tls_conf/certs/www.test2.com.crt",
                "ServerKeyFile" : "../conf/tls_conf/certs/www.test2.com.key"
            }
        } // SSL证书格式为nginx格式，而不是apache格式。
    }
}
```
* 源站1.1.1.1 添加iptables规则，只允许2.2.2.2 和3.3.3.3 访问tcp 8080端口
```
iptables -A INPUT -p icmp -j ACCEPT
iptables -A INPUT -i lo -j ACCEPT
iptables -A INPUT -p tcp -m multiport --dports 20,21,22,25,26,53,80,110,443 -j ACCEPT
iptables -A INPUT -p tcp --dport 8080 -s 2.2.2.2 -j ACCEPT
iptables -A INPUT -p tcp --dport 8080 -s 3.3.3.3 -j ACCEPT
iptables -A INPUT -p tcp --dport 8080 -j DROP
iptalbes -A INPUT -p udp -m multiport --dports 53,33333 -j ACCEPT
```