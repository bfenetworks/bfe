# 简介

host_rule.data是BFE的产品线域名表配置文件。

# 配置

| 配置项         | 类型   | 描述                                                         |
| -------------- | ------ | ------------------------------------------------------------ |
| Version        | String | 配置文件版本                                                 |
| DefaultProduct | String | 默认的产品线名称                                             |
| Hosts          | Map&lt;String, Array&lt;String&gt;&gt; | 域名标签和域名列表的映射关系，key是域名标签，value是域名列表 |
| HostTags       | Map&lt;String, Array&lt;String&gt;&gt; | 产品线和域名标签的映射关系，key是产品线名称，value是域名标签列表 |

# 示例

```
{
    "Version": "20190101000000",
    "DefaultProduct": null,
    "Hosts": {
        "exampleTag":[
            "example.org"
        ]
    },
    "HostTags": {
        "example_product":[
            "exampleTag"
        ]
    }
}
```
