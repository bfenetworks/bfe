# Introduction 

Block incoming connection/request based on defined rules.

# Module Configuration

## Description
conf/mod_block/mod_block.conf

| Config Item | Description | 
| ----------- | ----------- |
| Basic.ProductRulePath | path of product rule configuration |
| Basic.IPBlacklistPath | path of ip blacklist file |

## Example
```
[basic]
# product rule config file path
ProductRulePath = mod_block/block_rules.data

# global ip blacklist file path
IPBlacklistPath = mod_block/ip_blacklist.data
```

Format of IPBlacklistPath file

```
192.168.1.253 192.168.1.254
192.168.1.250
```

# Rule configuration

## Description

conf/mod_block/block_rules.data

| Config Item | Description                                                  |
| ----------- | ------------------------------------------------------------ |
| Version     | String<br>Verson of config file |
| Config      | Struct<br>Block rules for each product |
| Config{k}   | String<br>Product name |
| Config{v}   | Object<br>a list of rules |
| Config{v}[] | Object<br>a block rule |
| Config{v}[].Cond | String<br>Condition expression, See [Condition](../../condition/condition_grammar.md) |
| Config{v}[].Name | String<br>Name of rule |
| Config{v}[].Action | Object<br>Action of rule |
| Config{v}[].Action.Cmd | String<br>Name of action |
| Config{v}[].Action.Params | Object<br>a list of action parameters |
| Config{v}[].Action.Params[] | String<br>a action parameter |

## Actions
  
| Action | Description          |
| ------ | -------------------- |
| CLOSE  | Close the connection |
  
## Example

```
{
  "Version": "20190101000000",
  "Config": {
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

# Metrics

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

