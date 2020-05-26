# 集群间分流


## 概述

- 在BFE的[接入转发流程](./forward_model.md)中，在确定产品线后，要进一步确定处理该请求的目标集群
- 在BFE内，为每个产品线维护一张“转发表”
- 对每个属于该产品线的请求，查询转发表，获得目标集群


## 转发表

- 转发表由多条“转发规则”组成
- 每条转发规则包括两部分：匹配条件，目标集群
- 匹配条件使用“[条件表达式（Condition Expression）](../condition)”来表述
- 多条转发规则顺序执行。只要命中任何一条转发规则，就会结束退出
- 转发表必须包含“默认规则（Default）”。在所有转发规则都没有命中的时候，执行默认规则

## 示例

- 产品线demo，包含以下3种服务集群：
    + 静态集群(demo-static)：服务静态流量
    + post集群(demo-post)：服务post流量
    + main集群(demo-main)：服务其他流量
- 期望的转发条件如下：
    + 对于path以"/static"为前缀的，都发往demo-static集群
    + 请求方法为"POST"、且path以"/setting"为前缀的，都发往demo-post
    + 其它请求，都发往demo-main
- 对应以上要求，产品线demo的转发表如下图所示
![Forwarding Table](../../images/forwarding_table.png)





