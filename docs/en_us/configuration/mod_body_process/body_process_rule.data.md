# mod_body_process Rule Configuration

## Introduction

`body_process_rule.data` is the rule configuration file of the `mod_body_process` module, used to configure request/response body processing flows for each product line.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Version of the configuration file | Y | Usually a timestamp, e.g. `20190101000000` | Type is [Version](../00-common.md#5-version) |
| Config | Object | Body processing rule configuration for all product lines | Y | - | - |
| Config{k} | String | Product line name | Y | - | - |
| Config{v} | Array | Body processing rule list of the product line | Y | - | - |
| Config{v}[] | Object | A body processing rule | Y | - | - |
| Config{v}[].Cond | String | Matching condition | Y | Syntax see [Condition](../../condition/condition_grammar.md) | - |
| Config{v}[].RequestProcess | Object | Request body processing flow configuration | N | See data structure below | - |
| Config{v}[].ResponseProcess | Object | Response body processing flow configuration | N | See data structure below | - |

## Body Processing Flow Configuration

The data structure of the processing flow is as follows:

```go
// Processing flow
struct {
    Dec  string     // decoder; uses default if not specified
    Enc  string     // encoder; uses default if not specified
    Proc []ProcConf // processor list
}
// ProcConf
struct {
    Name   string   // processor name; currently only "textfilter" is supported
    Params []string // processor parameter list. textfilter: Params[0] - URL of the ToolGood.TextFilter service
}
```

Currently supported components:

### Decoder

- `line`: Parses data line by line; each line is an event
- `json`: Parses JSON objects from data; each JSON object is an event
- `sse`: Parses SSE events from data
- default: Automatically selects decoder based on contentType

### Processor

- `textfilter`: Calls the ToolGood.TextFilter service for content review

### Encoder

- default: Directly calls the event's ToBytes() function to generate body data

## Configuration Example

```json
{
    "Config": {
        "example_product": [
            {
                "Cond": "!req_body_json_in(\"model\", \"\", false)",
                "RequestProcess": {
                    "Proc": [
                        {"name": "textfilter", "params": ["http://172.19.1.136:9191/api/"]}
                    ]
                },
                "ResponseProcess": {
                    "Proc": [
                        {"name": "textfilter", "params": ["http://172.19.1.136:9191/api/"]}
                    ]
                }
            }
        ]
    },
    "Version": "20190101000000"
}
```
