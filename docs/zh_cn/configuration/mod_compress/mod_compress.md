# 简介 

mod_compress支持响应压缩，如：GZIP压缩。

# 配置

- 模块配置文件

  conf/mod_compress/mod_compress.conf

  ```
  [basic]
  DataPath = ../conf/mod_compress/compress_rule.data

  [log]
  OpenDebug = false
  ```

- 规则配置文件

  conf/mod_compress/compress_rule.data

  | Action                    | `描述`                        |
  | ------------------------- | ---------------------------- |
  | GZIP                      | gzip压缩。                    |

   ```
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
