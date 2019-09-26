# Concepts

- Product：
    - The product equals "tenant" in BFE, which has its own configuration, such as routing policy, permission, etc. 
    - Field "host" in HTTP header is used to identify product that the incoming message belongs to.

- Cluster：
    - Cluster means a set of backend servers which provide same functionality and process similar task.
    - Usually，cluster span multiple IDC/region in scope。

- Sub Cluster：
    - A cluster may be composed of multiple sub clusters conceptually。
    - Usally, backend servers within one IDC/region is defiend as a sub cluster.

- Instance：
    - Instance is backend server. A sub cluster would contain multiple instances.
    - To BFE，each backend instance expose IP address + port to accept request.

# Typical message routing flow

![Traffic Fowarding](../images/traffic-forward.svg)

- Step1-2：DNS query
    - request host name：demo.example.com
    - get IP address 6.6.6.6（example）

- Step3：IP diagram is routed to POP of IDC1，and processed by layer 4 load balancer L4LB.

- Step4：L4LB forward diagram to BFE

- Step5：BFE receive HTTP request, identify which product the request belongs to:
    - BFE uses field "host" in HTTP header to decide product.
    - In this scenario, assume demo.example.com belongs to product "demo".

- Step6：Identify a cluster of product "demo", which would handle this message.
    - Based on configured distribution rule of product "demo", identify cluster "demo-static" in this product to handle the message .

- Step7：Identify a sub cluster within "demo-static"
    - Based on routing rule in "demo-static", identify sub cluster "demo-static.idc1" to handle this message.

- Step8：Identify instance
    - based on configured load balancing policy, identify instance as "a-demo-static-1.idc1".

- Step9：request is sent to instance "a-demo-static-1.idc1".

- Step10：BFE recieve response message from "a-demo-static-1.idc1".

- Step11-12：BFE send back reponse to client, via L4LB.
