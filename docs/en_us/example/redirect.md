# Redirect

## Scenario

* Suppose our web server has been upgraded to https and we want to redirect http requests to https.
  * Domain：example.org

Add some configuration on the [basic routing configuration](route.md), we can implement redirect.

See the [complete configuration](../../../example_conf/redirect) for details

* Step 1. Enable mod_redirect
  * See the complete configuration [bfe.conf](../../../example_conf/redirect/bfe.conf) for details

```
+++ Modules = mod_redirect  (add a line of bfe.conf, enable mod_redirect)
```

* Step 2. Configure mod_redirect
  * configure data path of mod_redirect rules
  * configure [mod_redirect.conf](../../../example_conf/redirect/mod_redirect/mod_redirect.conf) as below

```
[basic]
DataPath = mod_redirect/redirect.data
```

* Step 3. Configure redirect rules
  * all http requests for example.org redirect to https
  * configure [redirect.data](../../../example_conf/redirect/mod_redirect/redirect.data) as below

```
{
    "Version": "init version",
    "Config": {
        "example_product": [{
            "Cond": "!req_proto_secure() && req_host_in(\"example.org\")",
            "Actions": [{
                "Cmd": "SCHEME_SET",
                "Params": [
                    "https"
                ]
            }],
            "Status": 301
        }]
    }
}
```

* Now, use curl to verify whether it can be redirected successfully.

curl -v -H "host: example.org" "http://127.1:8080/test"  will response 301，with location https://example.org/test

