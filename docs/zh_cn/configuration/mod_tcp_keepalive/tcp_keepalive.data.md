# mod_tcp_keepalive 规则配置

## 配置简介

`tcp_keepalive.data` 是 `mod_tcp_keepalive` 模块的规则配置文件，用于配置各产品线（租户）的 TCP 心跳包管理策略。

## 配置描述

| 配置项                            | 类型    | 参数含义                                                         | 必填 | 补充描述 | 合法性条件 |
| --------------------------------- | ------- | ---------------------------------------------------------------- | ---- | -------- | ---------- |
| Version                           | String  | 配置文件版本                                                     | Y    | -        | 类型为 [Version](../00-common.md#5-配置文件版本version) |
| Config                            | Object  | 各产品线（租户）的 TCP 心跳包管理规则                            | Y    | -        | -          |
| Config{k}                         | String  | 产品线名称                                                       | Y    | -        | -          |
| Config{v}                         | Array   | 产品线的规则列表                                                 | Y    | -        | -          |
| Config{v}[]                       | Object  | 某一条规则详细信息                                               | Y    | -        | -          |
| Config{v}[].VipConf               | Array   | 需要配置的 VIP（Virtual IP）数组，数组中的 VIP 共用以下同一套策略 | Y    | 数组元素为 IP 地址字符串 | 元素类型为 [IPAddr](../00-common.md#7-ip-地址ipaddr) |
| Config{v}[].KeepAliveParam        | Object  | TCP 长连接心跳包发送策略                                         | Y    | -        | -          |
| Config{v}[].KeepaliveParam.Disable | Boolean | 是否关闭心跳包发送                                               | N    | 默认值 `false` | -          |
| Config{v}[].KeepaliveParam.KeepIdle | Integer | 长连接中多长时间无数据发送后，开始发送心跳包                     | N    | 单位：秒 | 非负整数   |
| Config{v}[].KeepaliveParam.KeepIntvl | Integer | 如果上个心跳包未收到回应，多长时间后再次发送心跳包               | N    | 单位：秒 | 正整数     |
| Config{v}[].KeepaliveParam.KeepCnt | Integer | 心跳包未收到回应，再次发送心跳包的重试次数                       | N    | -        | 非负整数   |

## 配置示例

```json
{
    "Config": {
        "product1": [
            {
                "VipConf": ["10.1.1.1", "10.1.1.2"],
                "KeepAliveParam": {
                    "KeepIdle": 70,
                    "KeepIntvl": 15,
                    "KeepCnt": 9
                }
            },
            {
                "VipConf": ["10.1.1.3"],
                "KeepAliveParam": {
                    "Disable": true
                }
            }
        ],
        "product2": [
            {
                "VipConf": ["10.2.1.1"],
                "KeepAliveParam": {
                    "KeepIdle": 20,
                    "KeepIntvl": 15
                }
            }
        ]
    },
    "Version": "2021-06-25 14:31:06"
}
```
