# mod_auth_request

## Introduction

mod_auth_request supports sending request to the specified service for authentication.

## Module Configuration

### Description

conf/mod_auth_request/mod_auth_request.conf

| Config Item       | Description                                            |
| ----------------- | ------------------------------------------------------ |
| Basic.DataPath    | String<br>Path of rule configuration                   |
| Basic.AuthAddress | String<br>Address of authentication service            |
| Basic.AuthTimeout | Number<br>Timeout for authentication                   |
| Log.OpenDebug     | Boolean<br/>Whether enable debug log<br/>Default False |

### Example

```ini
[Basic]
DataPath = mod_auth_request/auth_request_rule.data
AuthAddress = http://127.0.0.1
AuthTimeout = 100

[Log]
OpenDebug = false
```

## Rule Configuration

### Description

| Config Item        | Description                                                  |
| ------------------ | ------------------------------------------------------------ |
| Version            | String<br>Version of config file                             |
| Config             | Object<br>Request auth rules for each product                |
| Config{k}          | String<br>Product name                                       |
| Config{v}          | Object<br> A list of request auth rules                      |
| Config{v}[]        | Object<br> A request auth rule                               |
| Config{v}[].Cond   | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].Enable | Boolean<br>Whether enable request auth rule                  |

### Example

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_in(\"/auth_request\", false)",
                "Enable": true
            }
        ]
    },
    Version": "20190101000000"
}
```

For example_product, for request to path /auth_request (e.g., www.example.com/auth_request), BFE will create a request and send it to http://127.0.0.1 for authentication.

### Actions

| Action | Condition                            |
| ------ | ------------------------------------ |
| Forbid | Response status code is 401 or 403   |
| Pass   | Response status code is 200 or other |

## Metrics

| Metric                    | Description                      |
| ------------------------- | -------------------------------- |
| AUTH_REQUEST_CHECKED      | Counter for checked request      |
| AUTH_REQUEST_PASS         | Counter for passed request       |
| AUTH_REQUEST_FORBIDDEN    | Counter for forbidden request    |
| AUTH_REQUEST_UNAUTHORIZED | Counter for unauthorized request |
| AUTH_REQUEST_FAIL         | Counter for failed request       |
| AUTH_REQUEST_UNCERTAIN    | Counter for uncertain request    |

## Illustration of how BFE create auth request

* Method: Request Method of HTTP Request created by BFE is **GET**
* Header: The request header created by the BFE is **originated from the original request**, but BFE makes following changes to the request:
  * Delete following headers: Content-Length/Connection/Keep-Alive/Proxy-Authenticate/Proxy-Authorization/Te/Trailers/Transfer-Encoding/Upgrade
  * Add following headers: X-Forwarded-Method(Original Request Method）、X-Forwarded-Uri（Original Request URI）
* Body: Body of HTTP Request created by BFE is **null**
