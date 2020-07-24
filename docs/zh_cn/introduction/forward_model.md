# 流量接入转发说明

![流量接入与转发](../../images/traffic-forward.png)

- Step 1-2：DNS解析
    - 请求的域名为 demo.example.com
    - 返回IP地址为 6.6.6.6（示例地址）

- Step 3：用户与 6.6.6.6:80 建立TCP连接并发送HTTP请求，IP报文被路由到IDC1的入口，由四层负载均衡设施处理

- Step 4：四层负载均衡设施将报文转发给下游BFE

- Step 5：BFE收到HTTP请求, 确定处理该请求的产品线
    - BFE根据HTTP请求头中的Host字段, 确定产品线
    - 对于demo.example.com域名，假设对应的产品线名为demo

- Step 6：BFE根据产品线的分流规则，选择该请求的目的集群
    - 对于这个请求，假设对应的目的集群为demo-static
    - 详见[基于内容路由](route.md)说明

- Step 7-8：BFE根据产品线的均衡策略，选择子集群及实例
    - 对于这个请求，假设子集群为demo-static.idc1，实例为demo-static-01.idc1
    - 详见[流量负载均衡](balance.md)说明

- Step 9：请求被发往后端实例demo-static-01.idc1

- Step 10：BFE收到后端实例回复的响应

- Step 11-12：BFE通过四层负载均衡设施，将响应返回给用户
