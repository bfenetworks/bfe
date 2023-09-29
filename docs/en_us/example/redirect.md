# Redirect

## Scenario

* Redirect HTTP to HTTPS for requests visiting example.org

## Connfiguration

Modify the example configurations (conf/) as the following steps:

* Step 1. modify conf/bfe.conf and enable mod_redirect

```ini
Modules = mod_redirect
```

* Step 2. modify mod_redirect basic configuration (conf/mod_redirect/mod_redirect.conf)
  
```ini
[basic]
DataPath = mod_redirect/redirect.data
```
  
* Step 3. modify redirect rule configuration (conf/mod_redirect/redirect.data), and add following rules.
  
```json
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
  
* Step 4. Verify configured rules

```bash
curl -H "host: example.org" "http://127.1:8080/test"  
```

The response stuatus code should be 301, and the value of Location response Header should be "https://example.org/test".
