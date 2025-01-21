# mod_block

## Introduction

mod_block blocks incoming connections/requests based on defined rules.

## Module Configuration

### Description

conf/mod_block/mod_block.conf

| Config Item | Description |
| ----------- | ----------- |
| Basic.ProductRulePath | Path of product rule configuration |
| Basic.IPBlocklistPath | Path of ip blocklist file |

### Example

```ini
[Basic]
# product rule config file path
ProductRulePath = mod_block/block_rules.data

# global ip blocklist file path
IPBlocklistPath = mod_block/ip_blocklist.data
```

Format of IPBlocklistPath file

```
192.168.1.253 192.168.1.254
192.168.1.250
```

## Rule Configuration

### Description

conf/mod_block/block_rules.data

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Version of config file |
| Config      | Struct<br>Block rules for each product |
| Config{k}   | String<br>Product name |
| Config{v}   | Object<br>A list of rules |
| Config{v}[] | Object<br>A block rule |
| Config{v}[].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].Name | String<br>Name of rule |
| Config{v}[].Action | Object<br>Action of rule |
| Config{v}[].Action.Cmd | String<br>Name of action |
| Config{v}[].Action.Params | Object<br>A list of action parameters |
| Config{v}[].Action.Params[] | String<br>A action parameter |

### Actions
  
| Action | Description          |
| ------ | -------------------- |
| CLOSE  | Close the connection |
| ALLOW  | Accept the request   |
  
### Example

```json
{
  "Version": "20190101000000",
  "Config": {
      "global": [
          {
              "action": {
                  "cmd": "ALLOW",
                  "params": []
              },
              "cond": "req_host_in(\"n.example.org\") && req_path_prefix_in(\"/index/\", false) && req_query_key_in(\"space\")",
              "name": "example whiterule"
          }
        ],
      "example_product": [
          {
            "action": {
                  "cmd": "CLOSE",
                  "params": []
              },
              "name": "example rule",
              "cond": "req_path_in(\"/limit\", false)"            
          }
      ]
  }
}
```

## Metrics

| Metric        | Description                                                  |
| ------------- | ------------------------------------------------------------ |
| CONN_ACCEPT   | Counter for connection accepted                              |
| CONN_REFUSE   | Counter for connection refused                               |
| CONN_TOTAL    | Counter for all connnetion checked                           |
| REQ_ACCEPT    | Counter for request accepted                                 |
| REQ_REFUSE    | Counter for request refused                                  |
| REQ_TOTAL     | Counter for all request in                                   |
| REQ_TO_CHECK  | Counter for request to check                                 |
| WRONG_COMMAND | Counter for request with condition satisfied, but wrong command |
