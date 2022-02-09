# mod_compress

## Introduction

mod_compress compresses responses based on specified rules.

## Module Configuration

### Description

conf/mod_compress/mod_compress.conf

| Config Item          | Description                                 |
| ---------------------| ------------------------------------------- |
| Basic.DataPath       | String<br>Path of rule configuration |
| Log.OpenDebug        | Boolean<br>Whether enable debug logs<br>Default False |

### Example

```ini
[Basic]
DataPath = mod_compress/compress_rule.data

[Log]
OpenDebug = false
```

## Rule Configuration

### Description

| Config Item | Description                                                |
| ----------- | -------------------------------------------------------------- |
| Version | String<br>Version of config file |
| Config | Object<br>Compress rule for each product |
| Config{k} | String<br>Product name |
| Config{v} | Object<br>A list of compress rules |
| Config{v}[] | Object<br>A compress rule |
| Config{v}[].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].Action | Object<br>Action |
| Config{v}[].Action.Cmd | String<br>Name of Action |
| Config{v}[].Action.Quality | Integer<br>Compression level |
| Config{v}[].Action.FlushSize | Integer<br>Flush size |

### Module Actions

| Action                  | Description                          |
| ------------------------| ------------------------------------|
| GZIP                    | Compress response using gzip method |
| BROTLI                  | Compress response using brotli method |

### Example

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "req_host_in(\"www.example.org\")",
                "Action": {
                    "Cmd": "GZIP",
                    "Quality": 9,
                    "FlushSize": 512
                }
            }
        ]
    },
    "Version": "20190101000000"
}
```
