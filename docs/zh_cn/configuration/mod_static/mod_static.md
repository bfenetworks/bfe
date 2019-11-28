# 简介 

mod_static支持静态文件访问。

# 配置

- 模块配置文件

  conf/mod_static/mod_static.conf

  ```
  [basic]
  DataPath = ../conf/mod_static/static_rule.data
  ```

- 规则配置文件

  conf/mod_static/static_rule.data

| Action                    | `描述`                             |
| ------------------------- | ---------------------------------- |
| BROWSE                    | 访问指定目录下的静态文件。 <br>第一个参数为根目录位置，第二个参数为默认静态文件名。|

   ```
    {
        "Config": {
            "example_product": [
                {
                    "Cond": "req_host_in(\"www.example.org\")",
                    "Action": {
                        "Cmd": "BROWSE",
                        "Params": [
                            "./",
                            "index.html"
                        ]
                    }
                }
            ]
        },
        "Version": "20190101000000"
    }
  ```
