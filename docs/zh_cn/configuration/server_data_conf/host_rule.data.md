# 域名规则配置

## 配置简介

host_rule.data是BFE的产品线域名表配置文件。

## 配置描述

| 配置项         | 描述                                   |
| -------------- | -------------------------------------- |
| Version        | String<br> 配置文件版本                |
| DefaultProduct | String<br>默认的产品线名称             |
| Hosts          | Object<br>域名标签和域名列表的映射关系 |
| Hosts{k}       | String<br>域名标签                     |
| Hosts{v}       | Object<br>域名列表                     |
| Hosts{v}[]     | String<br>域名信息                     |
| HostTags       | Object<br>产品线和域名标签的映射关系   |
| HostTags{k}    | String<br>产品线名称                   |
| HostTags{v}    | String<br>域名标签列表                 |
| HostTags{v}[]  | String<br>域名标签                     |

## 配置示例

```json
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
