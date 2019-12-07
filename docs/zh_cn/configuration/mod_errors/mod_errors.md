# 简介 

根据自定义的条件，将响应内容替换为/重定向至指定错误页。

# 配置

- 模块配置文件

  conf/mod_errors/mod_errros.conf

  ```
  [basic]
  DataPath = mod_errors/errors_rule.data
  ```

- 规则配置文件

  conf/mod_errors/errors_rule.data

| 配置项  | 类型   | 描述                                                         |
| ------- | ------ | ------------------------------------------------------------ |
| Version | String | 配置文件版本                                                 |
| Config  | Struct | 基于产品线的errors规则，每条规则包括：<br>- Cond: 描述匹配请求的条件<br/>- Actions: 匹配成功后的动作 |

| 动作     | `描述`                 |
| -------- | ---------------------- |
| RETURN   | 响应返回指定错误页     |
| REDIRECT | 响应重定向至指定错误页 |

  ```
{
    "Version": "20190101000000",
    "Config": {
        "example_product": [
            {
                "Cond": "res_code_in(\"404\")",
                "Actions": [
                    {
                        "Cmd": "RETURN",
                        "Params": [
                            "200", "text/html", "../conf/mod_errors/404.html"
                        ]
                    }
                ]
            },
            {
                "Cond": "res_code_in(\"500\")",
                "Actions": [
                    {
                        "Cmd": "REDIRECT",
                        "Params": [
                            "http://example.org/error.html"
                        ]
                    }
                ]
            }
        ]
    }
}
  ```
