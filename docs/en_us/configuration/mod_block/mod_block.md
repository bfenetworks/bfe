# Introduction 

Block incoming connection/request based on defined rules.

# Configuration

- Module config file

  conf/mod_block/mod_block.conf

  ```
  [basic]
  # product rule config file path
  ProductRulePath = ../conf/mod_block/block_rules.data
  
  # global ip blacklist file path
  IPBlacklistPath = ../conf/mod_block/ip_blacklist.data
  ```

- Data config file

  - ip blacklist file

    conf/mod_block/ip_blacklist.data

    ```
    192.168.1.253 192.168.1.254
    192.168.1.250
    ```

  - block rules file

    conf/mod_block/block_rules.data

| Config Item | Type   | Description                                                  |
| ----------- | ------ | ------------------------------------------------------------ |
| Version     | String | Verson of config file                                        |
| Config      | Struct | Block rules for each product. Block rule include: <br>- Cond: "condition" expression <br>- Action: what to do after matched<br>- Name: rule name |
  
| Action | Description          |
| ------ | -------------------- |
| CLOSE  | Close the connection |
  
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
  
  
