# mod_auth_request Basic Configuration

## Configuration Introduction

`mod_auth_request.conf` is the basic configuration file for the `mod_auth_request` module, used to specify the authentication rule file path, authentication service address, and timeout.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Basic.DataPath | String | Path of rule configuration | N | Default value is `mod_auth_request/auth_request_rule.data` | Type is [FilePath](../00-common.md#3-filepath); the file must exist and be readable |
| Basic.AuthAddress | String | Address of authentication service | Y | E.g., `http://127.0.0.1` | Must be a valid URL |
| Basic.AuthTimeout | Integer | Timeout for authentication | Y | Unit: milliseconds | Must be greater than 0 |
| Log.OpenDebug | Boolean | Debug flag of module | N | Default value is `false` | - |

## Configuration Example

```ini
[Basic]
DataPath = mod_auth_request/auth_request_rule.data
AuthAddress = http://127.0.0.1
AuthTimeout = 100

[Log]
OpenDebug = false
```
