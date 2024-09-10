# mod_cors

## Introduction

mod_cors support Cross-Origin Resource Sharing

## Module configuration

### Description

conf/mod_cors/mod_cors.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.DataPath | String<br>Path of rule configuration |
| Log.OpenDebug  | Boolean<br>Debug flag of module      |

### Example

```ini
[Basic]
DataPath = mod_cors/cors_rule.data

[Log]
OpenDebug = false
```

## Rule Configuration

### Description

conf/mod_cors/cors_rule.data

| Config Item                | Description                             |
| -------------------------- | -------------------------------------------- |
| Version                    | String<br>Version of the config file         |
| Config                     | Object<br>Trace rules for each product      |
| Config[k]                  | String<br>Product name                      |
| Config[v]                  | Object<br>A list of cors rules     |
| Config[v][]                | Object<br>A cors rule                     |
| Config[v][].Cond           | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config[v][].AccessControlAllowOrigins    | List<br> Indicates whether the response can be shared with requesting code from the given origin; for requests without credentials, the "*" wildcard, to tell browsers to allow any origin to access the resource. "%origin" specifies the origin from the request header "Origin" |
| Config[v][].AccessControlAllowCredentials| Boolean<br> Indicates whether or not the response to the request can be exposed.|
| Config[v][].AccessControlExposeHeaders   | Boolean<br> Specifies the response headers that browsers are allowed to access. |
| Config[v][].AccessControlAllowMethods    | List<br> Specifies the method or methods allowed when accessing the resource. This is used in response to a preflight request.|
| Config[v][].AccessControlAllowHeaders    | List<br> Indicates which HTTP headers can be used when making the actual request. This is used in response to a preflight request.|
| Config[v][].AccessControlMaxAge          | Int<br>Indicates how long the results of a preflight request can be cached. This is used in response to a preflight request.|

### Example

```json
{
    "Version": "cors_rule.data.version",
    "Config": {
        "example_product": [
             {
                 "Cond": "req_host_in(\"example.org\")",
                 "AccessControlAllowOrigins": ["%origin"],
                 "AccessControlAllowCredentials": true,
                 "AccessControlExposeHeaders": ["X-Custom-Header"],
                 "AccessControlAllowMethods": ["HEAD","GET","POST","PUT","DELETE","OPTIONS","PATCH"],
                 "AccessControlAllowHeaders": ["X-Custom-Header"],
                 "AccessControlMaxAge": -1
             }
         ]
    }
}
```
