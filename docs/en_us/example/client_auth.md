# TLS client authentication

## Scenario

* The server needs to authenticate the client using TLS client authentication

## Configuration steps

* Step 1. Generate root certificate

```bash
openssl genrsa -out root.key 2048

openssl req -new -x509 -days 365 -key root.key -out root.crt
```

* Step 2. Create a client certificate signing request

```bash
openssl genrsa -out client.key 2048

openssl req -new -out client.csr -key client.key  
```

* Step 3. Generate client certificate

```bash
echo "extendedKeyUsage = clientAuth" > openssl.cnf

openssl x509 -req -in client.csr -out client.crt -signkey client.key -CA root.crt -CAkey root.key  -days 365  -extfile openssl.cnf
```

* Step4. Configure layer 4 load balancing service.
In this example, HAproxy is used as the layer 4 load balancing service, and VIP is passed to BFE using PROXY protocol.
HAproxy can be installed through "apt install haproxy" on Ubuntu system. For more details, see [www.haproxy.org](http://www.haproxy.org).
  
Configuration file(haproxy.cfg) example：

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

Run HAproxy

```bash
haproxy -f haproxy.cfg
```

* Step 5. Configure BFE.
Copy root.crt to tls_conf/client_ca directory(note: the suffix of root certificate should be ".crt").

```ini
[server]
...
Layer4LoadBalancer = "PROXY"
...

[HttpsBasic]
...
clientCABaseDir = tls_conf/client_ca
...
```
  
Modify conf/tls_conf_rule.data and set "ClientAuth" to true and "ClientCAName" to name of the root certificate.
  
```json
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

Run BFE.

```bash
./bfe -c ../conf
```

* Step 6. Verify configuration

```bash
openssl s_client -connect 127.0.0.1:7443 -cert client.crt -key client.key -state -quiet
```
