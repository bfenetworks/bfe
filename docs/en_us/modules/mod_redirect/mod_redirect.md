# mod_redirect

## Introduction 

Redirect HTTP request based on defined rules.

## Module Configuration

### Description
conf/mod_redirect/mod_redirect.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.DataPath | String<br>path of rule configuraiton |

### Example

```
[basic]
DataPath = mod_redirect/redirect.data
```

## Rule Configuration

### Description
conf/mod_redirect/redirect.data

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Verson of config file                                        |
| Config      | Struct<br>Redirect rules for each product. |
| Config{k}   | String<br>Product name |
| Config{v}   | Object<br>A ordered list of redirect rules |
| Config{v}[] | Object<br>A redirect rule |
| Config{v}[].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].Actions | Object<br>A ordered list of redirect actions |
| Config{v}[].Actions[] | Object<br>A redirect action |
| Config{v}[].Actions[].Cmd | Object<br>Name of redirect action |
| Config{v}[].Actions[].Params | Object<br>Parameters of redirect action |
| Config{v}[].Status | Integer<br>Status code |

### Actions
| Action         | Description                                                                         |
| -------------- | ----------------------------------------------------------------------------------- |
| URL_SET        | redirect to specified URL                                                           |
| URL_FROM_QUERY | redirect to URL parsed from specified query in request                              |
| URL_PREFIX_ADD | redirect to URL concatenated by specified prefix and the original URL               |
| SCHEME_SET     | redirect to the original URL but with scheme changed. supported scheme: http\|https |
  
### Example

```
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "req_path_prefix_in(\"/redirect\", false)",
                "Actions": [
                    {
                        "Cmd": "URL_SET",
                        "Params": ["https://example.org"]
                    }
                ],
                "Status": 301
            }
        ]
    }
}
```
