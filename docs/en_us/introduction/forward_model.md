# Traffic forwarding

![Traffic Forwarding](../../images/traffic-forward.svg)

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

- Step10：BFE receive response message from "a-demo-static-1.idc1".

- Step11-12：BFE send back response to client, via L4LB.
