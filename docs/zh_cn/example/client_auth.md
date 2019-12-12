# TLS客户端认证

## 场景说明

* 假设我们需要使用TLS双向认证机制对客户端进行认证。

## 配置步骤

* 生成根证书与客户端证书。

  * 生成根证书

  ```
  openssl genrsa -out root.key 2048

  openssl req -new -x509 -days 365 -key root.key -out root.crt
  ```

  * 创建客户端证书签名申请

  ```
  openssl genrsa -out client.key 2048

  openssl req -new -out client.csr -key client.key  
  ```

  * 生成客户端证书

  ```
  echo "extendedKeyUsage = clientAuth" > openssl.cnf

  openssl x509 -req -in client.csr -out client.crt -signkey client.key -CA root.crt -CAkey root.key  -days 365  -extfile openssl.cnf
  ```

* 配置4层负载均衡服务（客户端认证针对VIP启用，配置4层负载均衡服务是为了获取VIP），这里使用HAproxy作为4层负载均衡服务，使用PROXY协议将VIP传递给BFE。示例中，HAproxy与BFE同机部署。

  * 安装HAproxy，下载[www.haproxy.org](http://www.haproxy.org)。Ubuntu系统可通过apt install haproxy安装。

  * 配置HAproxy，配置文件（haproxy.cfg）示例：

  ```
    global

    defaults
            mode    tcp
            balance leastconn
            timeout client      3000ms
            timeout server      3000ms
            timeout connect     3000ms

    frontend fr_server_http
            bind 0.0.0.0:7080
            default_backend bk_server_http

    backend bk_server_http
            server srv1 0.0.0.0:8080 maxconn 2048 send-proxy

    frontend fr_server_https
            bind 0.0.0.0:7443
            default_backend bk_server_https

    backend bk_server_https
            server srv1 0.0.0.0:8443 maxconn 2048 send-proxy
  ```

  * 启动HAproxy

  ```
  haproxy -f haproxy.cfg
  ```

* 配置BFE。
  * 配置客户端证书文件存储路径（[bfe.conf](../../../conf/bfe.conf)），将root.crt复制到tls_conf/client_ca目录（注：根证书文件后缀名必须为.crt）。

  ```
  [server]
  ...
  Layer4LoadBalancer = "PROXY"
  ...

  [HttpsBasic]
  ...
  clientCABaseDir = tls_conf/client_ca
  ...
  ```
  
  * 修改[tls_conf_rule.data](../../../conf/tls_conf/tls_rule_conf.data)，将ClientAuth置为true，ClientCAName填写根证书文件名。
  
  ```
    {
        "Version": "12",
        "DefaultNextProtos": [
            "http/1.1"
        ],
        "Config": {
            "example_product": {
                "VipConf": [
                    "127.0.0.1"
                ],
                "SniConf": null,
                "CertName": "example.org",
                "NextProtos": [
                    "h2;rate=0;isw=65535;mcs=200;level=0",
                    "http/1.1"
                ],
                "Grade": "C",
                "ClientAuth": true,
                "ClientCAName": "root"
            }
        }
    }
  ```
  * 启动BFE。

* 现在，用openssl验证下是否成功。

  ```
  openssl s_client -connect 127.0.0.1:7443 -cert client.crt -key client.key -state -quiet
  ```