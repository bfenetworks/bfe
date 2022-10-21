# Rewrite

## Scenario

* The service API '/service' is upgraded to '/v1/service' during evolution.
* To avoid from breaking existing clients, bfe rewrites requests with path '/service' and then forward to the backend service.

## Configuration

Modify example configurations (conf/) as the following steps:

* Step 1. Modify conf/bfe.conf and enable mod_rewrite

```ini
Modules = mod_rewrite  # enable mod_rewrite
```

* Step 2. Modify conf/mod_rewrite/mod_rewrite.conf and set the rule configuration file

```ini
[basic]
DataPath = mod_rewrite/rewrite.data
```

* Step 3. Modify rewrite rules configuration

```json
{
    "Version": "init version",
    "Config": {
        "example_product": [{
            "Cond": "req_path_prefix_in(\"/service\", false)",
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

* Step 4. Verify configured rules

```bash
curl -H "host: example.org" "http://127.1:8080/service"
```

The final path of request received by service 'cluster_demo_dynamic' is 'v1/service'.
