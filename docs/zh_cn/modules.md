# 扩展模块说明

## rewrite

### 功能

根据自定义的条件，修改请求的URI：
- 如果命中了rewrite规则的条件
- 在请求转发时，会执行rewrite动作

### 配置

- rewrite规则包括：
    - 条件描述：使用condition来描述匹配的条件
    - 操作：定义了rewrite的操作
    - 命中后中止：如果为true，则在本条规则命中后，不再继续尝试匹配其他的规则

- 多条规则之间是有序的
    - 会按照规则顺序逐条尝试匹配

## redirect

### 功能

根据自定义的条件，对请求进行重定向

### 配置

- 每条规则包括：
    - 条件描述：使用condition来描述匹配的条件
    - 操作：定义了redirect的操作
    - status：定义了HTTP响应返回码

- 多条规则之间是有序的
    - 会按照顺序逐条尝试匹配

## header

### 功能

根据自定义条件，修改请求或响应的头部，比如：

- 在请求中增加header以标识用户真实IP地址
- 根据配置的条件，rewrite消息中的header

### 配置

- 模块配置文件

    conf/mod_header/mod_header.conf

    ```
    [basic]

    DataPath = ../conf/mod_header/header_rule.data
    ```

- 规则配置文件

    conf/mod_header/header_rule.data

## block

### 功能

- 设置访问控制条件，以禁止符合该条件的请求, 例如：
 * 支持IP黑名单功能，禁止来自某些网段的请求
- 如果访问被禁止，连接直接被关闭

### 配置

- 模块配置文件

    conf/mod_block/mod_block.conf
    ```
    [basic]

    # product rule config file path

    ProductRulePath = ../conf/mod_block/block_rules.data

    # global ip blacklist file path

    IPBlacklistPath = ../conf/mod_block/ip_blacklist.data
    ```
- 数据配置文件

    - 黑名单文件

        conf/mod_block/ip_blacklist.data

    - 封禁规则文件

        conf/mod_block/block_rules.data

## logid

### 功能

- 在转发的请求中添加一个头部 "X-Bfe-Log-Id"，用于携带BFE生成的全局唯一的logid。

- 生成算法
    - 对以下参数进行hash
        - 协议类型，tcp 或udp
        - 源地址，ip:port
        - 目的地址，ip:port
        - BFE进程pid
        - 当前时间

## trust_clientip

### 功能

配置信任IP列表，标识请求的真实用户IP是否属于信任IP列表中。

### 配置

- 模块配置文件

    conf/mod_trust_clientip/mod_trust_clientip.conf

    ```
    [basic]

    DataPath = ../conf/mod_trust_clientip/trust_client_ip.data
    ```

- 信任IP数据配置文件

    conf/mod_trust_clientip/trust_clients_ip.data

