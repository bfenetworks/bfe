# Introduction 

Modify header of HTTP request/response based on defined rules.

# Configuration

- Module config file

  conf/mod_header/mod_header.conf

  ```
  [basic]
  DataPath = ../conf/mod_header/header_rule.data
  ```

- Rule config file

  conf/mod_header/header_rule.data

| Config Item | Type   | Description                                                  |
| ----------- | ------ | ------------------------------------------------------------ |
| Version     | String | Verson of config file                                        |
| Config      | Struct | Header rules for each product. Header rule include: <br>- Cond: "condition" expression <br>- Actions: what to do after matched<br>- Last: if true, stop to check the remaining rules |

| Action         | Description            |
| -------------- | ---------------------- |
| REQ_HEADER_SET | Set request header     |
| REQ_HEADER_ADD | Add request header     |
| RSP_HEADER_SET | Set response header    |
| RSP_HEADER_ADD | Add response header    |
| REQ_HEADER_DEL | Delete request header  |
| RSP_HEADER_DEL | Delete response header |
| REQ_HEADER_MOD | Modify request header  |
| RSP_HEADER_MOD | Modify response header |

  ```
  {
      "Version": "20190101000000",
      "Config": {
          "example_product": [
              {
                  "cond": "req_path_prefix_in(\"/header\", false)",
                  "actions": [
                      {
                          "cmd": "RSP_HEADER_SET",
                          "params": [
                              "X-Proxied-By",
                              "bfe"
                          ]
                      }
                  ],
                  "last": true
              }
          ]
      }
  }
  ```

  
