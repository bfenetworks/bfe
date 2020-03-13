# Introduction 

Add user id to cookie for client identification.

# Configuration

- Module config file

  conf/mod_userid/mod_userid.conf

  ```
  [basic]
  DataPath = mod_userid/userid_rule.data

  [Log]
  OpenDebug = true
  ```

- Rule config file

  conf/mod_userid/userid_rule.data

  | Config Item | Type   | Description                                             |
  | ----------- | ------ | ------------------------------------------------------- |
  | Version     | String | Verson of config file                                   |
  | Products    | Map    | key is product name,value is array, elememt is one rule |
  
  Products[\$product_name].[$index] config:
  

  | Config Item   | Description            |
  | ------------- | ---------------------- |
  | Cond          | "condition" expression |
  | Params.Name   | the cookie name        |
  | Params.Domain | the cookie domain      |
  | Params.Path   | the cookie path        |
  | Params.MaxAge | the cookie max age     |

  ```
  {
      "Version": "2019-12-10184356",
      "Products": {
          "example_product": [
              {
                  "Cond": "req_path_prefix_in(\"/abc\", true)",
                  "Params": {
                       "Name": "bfe_userid_abc",
                       "Domain": "",
                       "Path": "/abc",
                       "MaxAge": 3153600
                   },
                   "Generator": "default"
              }, 
              {
                  "Cond": "default_t()",
                  "Params": {
                       "Name": "bfe_userid",
                       "Domain": "",
                       "Path": "/",
                       "MaxAge": 3153600
                   }
              }
          ]
      }
  }
  ```

  

