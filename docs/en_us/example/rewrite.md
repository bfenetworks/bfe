# Rewrite

## Scenario

* Assuming that URL PATH has changed after our service upgraded and some APPs that have been released cannot be modified. 
* We hope that the requests for the old path can automatically modify to new PATH, instead of maintaining two sets of service paths.
  * Old PATH：/service
  * New PATH：/v1/service

Add some configuration on the [basic routing configuration](route.md), you can implement rewrite.

See the [complete configuration](../../../example_conf/rewrite) for details

* Step 1. Enable mod_rewrite
  * See the complete configuration [bfe.conf](../../../example_conf/rewrite/bfe.conf) for details

```
+++ Modules = mod_rewrite  (add a line of bfe.conf, enable mod_rewrite)
```

* Step 2. Configure mod_rewrite
  * configure data path of mod_rewrite rules
  * configure [mod_rewrite.conf](../../../example_conf/rewrite/mod_rewrite/mod_rewrite.conf) as below

```
[basic]
DataPath = mod_rewrite/rewrite.data
```

* Step 3. Configure rewrite rules
  * all http requests for example.org will be forwarded to backends after adding /v1 prefix.
  * configure [rewrite.data](../../../example_conf/rewrite/mod_rewrite/rewrite.data) as below

```
{
    "Version": "init version",
    "Config": {
        "example_product": [{
            "Cond": "req_host_in(\"example.org\")",
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

* Now, use curl to verify whether it can be rewritten successfully.

curl -H "host: example.org" "http://127.1:8080/service", request received by cluster_demo_dynamic will be changed to "/v1/service"
