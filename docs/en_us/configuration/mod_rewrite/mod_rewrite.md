# Introduction 

Modify URI of HTTP request based on defined rules.

# Configuration

- Module config file

  conf/mod_rewrite/mod_rewrite.conf

  ```
  [basic]
  DataPath = ../conf/mod_rewrite/rewrite.data
  ```

- Rule config file

  conf/mod_rewrite/rewrite.data

| Config Item | Type   | Description                                                  |
| ----------- | ------ | ------------------------------------------------------------ |
| Version     | String | Verson of config file                                        |
| Config      | Struct | Rewrite rules for each product. Rewrite rule include: <br>- Cond: "condition" expression <br>- Actions: what to do after matched<br>- Last: if true, stop to check the remaining rules |
  
| Action                    | Description                              |
| ------------------------- | ---------------------------------------- |
| HOST_SET                  | Set host to specified value              |
| HOST_SET_FROM_PATH_PREFIX | Set host to specified path prefix        |
| PATH_SET                  | Set path to specified value              |
| PATH_PREFIX_ADD           | Add prefix to orignal path               |
| PATH_PREFIX_TRIM          | Trim prefix from orignal path            |
| QUERY_ADD                 | Add query                                |
| QUERY_DEL                 | Delete query                             |
| QUERY_DEL_ALL_EXCEPT      | Del all queries except specified queries |
| QUERY_RENAME              | Rename query                             |
  
  ```
  {
      "Version": "20190101000000",
      "Config": {
          "example_product": [
              {
                  "Cond": "req_path_prefix_in(\"/rewrite\", false)",
                  "Actions": [
                      {
                          "Cmd": "PATH_PREFIX_ADD",
                          "Params": [
                              "/bfe/"
                          ]
                      }
                  ],
                  "Last": true
              }
          ]
      }
  }
  ```
  
