# mod_errors

## Introduction

mod_errors replaces error responses based on specified rules.

## Module Configuration

### Description

conf/mod_errors/mod_errors.conf

| Config Item          | Description                                 |
| ---------------------| ------------------------------------------- |
| Basic.DataPath       | String<br>Path for rule configuration |
| Log.OpenDebug        | Boolean<br>Whether enable debug logs<br>Default False |

### Example

```ini
[Basic]
DataPath = mod_errors/errors_rule.data
```

## Rule Configuration

### Description

| Config Item | Description                                                |
| ----------- | ---------------------------------------------------------- |
| Version | String<br>Version of config file |
| Config | Object<br>Error rules for each product |
| Config{k} | String<br>Product name |
| Config{v} | Object<br> A list of error rules |
| Config{v}[] | Object<br>A error rule |
| Config{v}[].Cond | String<br>Condition expressio, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].Actions | Object<br>Action |
| Config{v}[].Actions.Cmd | String<br>Name of Action |
| Config{v}[].Actions.Params | Object<br>Parameters of Action |
| Config{v}[].Actions.Params[] | String<br>A Parameter |

### Module Actions

| Action   | Description            |
| -------- | ---------------------- |
| RETURN   | Return response generated from specified static html |
| REDIRECT | Redirect to specified location |

### Example

```json
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "res_code_in(\"404\")",
                "Actions": [
                    {
                        "Cmd": "RETURN",
                        "Params": [
                            "200", "text/html", "../conf/mod_errors/404.html"
                        ]
                    }
                ]
            },
            {
                "Cond": "res_code_in(\"500\")",
                "Actions": [
                    {
                        "Cmd": "REDIRECT",
                        "Params": [
                            "http://example.org/error.html"
                        ]
                    }
                ]
            }
        ]
    }
}
```
