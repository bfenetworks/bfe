# BFE流量接入转发流程

![流量接入与转发](../../introdutionn/images/traffic-forward.svg)

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
