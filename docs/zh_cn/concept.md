# 概念说明

- 产品线（Product）：
    - 产品线即为BFE中的"租户"。BFE中的配置，比如转发的策略、权限等，是以产品线为单位来进行设置的。
    - HTTP header中的host字段（即域名），决定该消息由哪个产品线处理。

- 集群(Cluster)：
    - 具有同类功能的后端的集合定义为一个集群。一个产品线中可定义多个集群。
    - 通常，一个集群的范围可能跨越多个IDC。

- 子集群(Subcluster)：
    - 集群又可以划分为多个子集群。
    - 通常，将集群中处于同一IDC中的后端定义为一个子集群。

- 实例（Instance）：
    - 每个子集群可包含多个后端服务实例。
    - 对于BFE，每个后端实例表现为"IP地址 + 端口号"。

# 典型转发流程

BFE转发和处理流程

![](../images/traffic-routing.svg)

- Step1-2：DNS解析
    - 请求的域名为demo.example.com
    - 返回IP地址6.6.6.6（示例地址）

- Step3：IP报文被路由到IDC1的入口，由四层负载均衡系统L4LB处理

- Step4：L4LB将IP报文转发给下游BFE

- Step5：BFE收到HTTP request, 确定处理该消息的产品线
    - BFE根据HTTP header中的host字段确定产品线。
    - 对于demo.example.com这个域名，假设对应的产品线名为demo

- Step6：确定产品线demo中处理该消息的集群（具体参见“集群间分流”）
    - 查找产品线demo的配置，根据配置的集群间分流规则，确定后端集群为demo-static

- Step7：根据集群demo-static的配置，确定子集群
    - 对这个请求，确定子集群为demo-static.idc1

- Step8：在子集群内，确定实例
    - 对这个请求，确定实例为a-demo-static-1.idc1

- Step9：请求被发往目标实例a-demo-static-1.idc1

- Step10：BFE收到Response

- Step11-12：BFE通过L4LB，将请求返回给用户
