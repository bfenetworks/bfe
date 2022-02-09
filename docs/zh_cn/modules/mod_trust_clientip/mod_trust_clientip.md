# mod_trust_clientip

## 模块简介

mod_trust_clientip基于配置信任IP列表，检查并标识访问用户真实IP是否属于信任IP。

## 基础配置

### 配置描述

模块配置文件: conf/mod_trust_clientip/mod_trust_clientip.conf

| 配置项         | 描述                             |
| -------------- | -------------------------------- |
| Basic.DataPath | String<br>IP字典文件路径，包含了所有信任IP |

### 配置示例

```ini
[Basic]
DataPath = mod_trust_clientip/trust_client_ip.data
```

## 字典配置

### 配置描述

字典配置文件路径: conf/mod_trust_clientip/trust_client_ip.data

| 配置项            | 描述                            |
| ----------------- | ------------------------------- |
| Version           | String<br>配置文件版本          |
| Config            | Object<br>所有信任的IP列表      |
| Config[k]         | String<br>地址标签              |
| Config[v]         | Object<br>信任的IP段列表        |
| Config[v][]       | Object<br>IP段                  |
| Config[v][].Begin | String<br>IP段起始地址          |
| Config[v][].End   | String<br>IP段结束地址          |

### 配置示例

```json
{
    "Version": "20190101000000",
    "Config": {
        "inner-idc": [
            {
                "Begin": "10.0.0.0",
                "End": "10.255.255.255"
            }
        ]
    }
}
```

## 监控信息

| 监控项                       | 描述                                   |
| ---------------------------- | -------------------------------------- |
| CONN_TOTAL                   | 所有连接数                             |
| CONN_TRUST_CLIENTIP          | 来源于信任地址的连接数                 |
