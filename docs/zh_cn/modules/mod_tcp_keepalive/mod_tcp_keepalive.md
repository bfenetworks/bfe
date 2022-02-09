# mod_tcp_keepalive

## 模块简介

mod_tcp_keepalive管理TCP长连接心跳包的发送策略。

在某些场景下可能对耗电量十分敏感，比如智能手表待机状态，希望能够停止发送TCP长连接时的心跳包，或者降低其发送频率，此模块即可以用来处理此类或者其他需要管理TCP心跳包发送策略的场景。

## 基础配置

### 配置描述

模块配置文件: conf/mod_tcp_keepalive/mod_tcp_keepalive.conf

| 配置项 | 描述 |
| ----- | --- |
| Basic.DataPath | String<br> 规则文件路径 |
| Log.OpenDebug | Boolean<br>是否开启debug模式 |

### 配置示例

```ini
[Basic]
DataPath = ../data/mod_tcp_keepalive/tcp_keepalive.data

[Log]
OpenDebug = false
```

## 规则配置

### 配置描述

| 配置项 | 描述 |
| ----- | --- |
| Version | String<br>配置文件版本 |
| Config | Object<br>各产品线（租户）的TCP心跳包管理规则 |
| Config{k} | String<br>产品线名称 |
| Config{v} | Array<br>产品线的规则列表 |
| Config{v}[] | Object<br>某一条规则详细信息 |
| Config{v}[].VipConf | Array<br>需要配置的VIP（Virtual IP）数组，数组中的VIP共用以下同一套策略 |
| Config{v}[].KeepAliveParam | Object<br>TCP长连接心跳包发送策略 |
| Config{v}[].KeepaliveParam.Disable | Bool<br>是否关闭心跳包发送，默认false |
| Config{v}[].KeepaliveParam.KeepIdle | Int<br>长连接中多长时间无数据发送后，开始发送心跳包 |
| Config{v}[].KeepaliveParam.KeepIntvl | Int<br>如果上个心跳包未收到回应，多长时间后再次发送心跳包 |
| Config{v}[].KeepaliveParam.KeepCnt | Int<br>心跳包未收到回应，再次发送心跳包的重试次数 |

### 配置示例

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

## 监控项

| 监控项        | 描述                         |
| ------------- | ---------------------------- |
| CONN_TO_SET    | 命中配置规则的连接总数                     |
| CONN_SET_KEEP_IDLE | 设置keepIdle属性的连接数 |
| CONN_SET_KEEP_IDLE_ERROR | 设置keepIdle属性失败的连接数 |
| CONN_SET_KEEP_INTVL | 设置keepIntvl属性的连接数 |
| CONN_SET_KEEP_INTVL_ERROR | 设置keepIntvl属性失败的连接数 |
| CONN_SET_KEEP_CNT | 设置keepCnt属性的连接数 |
| CONN_SET_KEEP_CNT_ERROR | 设置keepCnt属性失败的连接数 |
| CONN_DISABLE_KEEP_ALIVE | 设置disable属性的连接数 |
| CONN_DISABLE_KEEP_ALIVE_ERROR | 设置disable属性失败的连接数 |
| CONN_COVERT_TO_TCP_CONN_ERROR | 将连接类型转换为TCPConn类型失败的次数 |
