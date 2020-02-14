# 简介

添加用户标识，供产品线和客户端使用。

# 配置

- 模块配置文件

  conf/mod_userid/mod_userid.conf

  ```
  [basic]
  DataPath = mod_userid/userid_rule.data

  [Log]
  OpenDebug = true
  ```

- 规则配置文件

  conf/mod_userid/userid_rule.data

  | 配置项   | 类型   | 描述                                                   |
  | -------- | ------ | ------------------------------------------------------ |
  | Version  | String | 配置文件版本                                           |
  | Products | Map    | key 是产品线的名字，值是一个数组，每个元素表示一条规则 |
  
  Products[\$product_name].[$index] config:
  

  | 配置项        | 描述                   |
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
