# 域名规则配置

## 配置简介

host_rule.data是BFE的产品线域名表配置文件。

## 配置描述

| 配置项         | 类型     | 参数含义                       | 必填 | 补充描述                                                     | 合法性条件                                                   |
| -------------- | -------- | ------------------------------ | ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Version        | String   | 配置文件版本                   | Y    | 参见 [Version](../00-common.md#5-配置文件版本version) 类型定义  | 类型为 [Version](../00-common.md#5-配置文件版本version)         |
| DefaultProduct | String   | 默认的产品线名称               | N    | 无默认值；未配置时无默认产品线                               | -                                                            |
| Hosts          | Object   | 域名标签和域名列表的映射关系   | Y    | 键为域名标签，值为域名列表                                   | 非空                                                         |
| Hosts{k}       | String   | 域名标签                       | Y    | 作为 Hosts 的键                                              | 非空                                                         |
| Hosts{v}       | []String | 域名列表                       | Y    | 该标签下的所有域名                                           | 非空                                                         |
| Hosts{v}[]     | String   | 域名信息                       | Y    | 具体域名                                                     | 非空；须为有效域名或主机名                                   |
| HostTags       | Object   | 产品线和域名标签的映射关系     | Y    | 键为产品线名称，值为域名标签列表                             | 非空                                                         |
| HostTags{k}    | String   | 产品线名称                     | Y    | 作为 HostTags 的键                                           | 非空                                                         |
| HostTags{v}    | []String | 域名标签列表                   | Y    | 该产品线关联的所有域名标签                                   | 非空                                                         |
| HostTags{v}[]  | String   | 域名标签                       | Y    | 须与 Hosts 中定义的某个域名标签一致                          | 非空；须在 Hosts 中存在                                      |

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
