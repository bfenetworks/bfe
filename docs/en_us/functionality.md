# Request Routing between clusters

In BFE configuration, a "product" could be composed of multiple clusters. User can define how requests are routed among the clusters. Request routing is based on the content of HTTP request.

## Rule configuration

- Fields in HTTP Header is used to define routing rule for distributing traffic in cluster level, for example:
    - host, path, query, cookie, method, etc.
- BFE provide a "Condition" expression to define how to route message with special header. This is routing rule in cluster level load balance.
- If multiple rules are configured, BFE would match the rules in sequence. Matching procedure stop if one rule is matched.

## Example

- A product "demo", which needs process three kinds of traffic: static content traffic, "post" traffic, other traffic, then we can define three clusters ：

    - demo-static：serve static content 
    - demo-post：serve post message
    - demo-main: serve other traffic

- To BFE configuration, following routing rules can be added:
    - Rule 1: req_path_prefix_in("/static", false) -> demo-static, which means messages with path prefixed with "/static" will be routed to cluster demo-static.
    - Rule 2: req_method_in("POST")&&req_path_prefix_in("/setting",false) -> demo-post, which means that message which use method "POST" and is prefixed with "/setting" will be routed to cluster "demo-post". 
    - Rule 3: default -> demo-main, which means all message which doesn't match above rules will be sent to cluster "demo-main".

# Sub cluster level load balance

- In sub cluster level, load balance rules also can be configured. The rules define traffic weight which is distributed to each sub cluster。

- A special virtual sub cluster "BLACKHOLE" could be used to discard traffic。

## Example

- Consider following configuration：
    - Two IDC：IDC1, IDC2
    - Two BFE cluster：BFE1, BFE2
    - Two backend sub cluster：sub-cluster-1, sub-cluster-2

- In BFE cluster(BFE1 and BFE2):
    - BFE1：{sub-cluster-1:w11，sub-cluster-2:w12, Blackhole:w1B}
    - BFE2：{sub-cluster-1:w21，sub-cluster-2:w22, Blackhole:w2B}

- Based on the weight in configuration, BFE distribute traffic to backend sub cluster.
    - For example，if weight configuration {w11, w12, w1B} ={45，45，10}, percent of traffic to sub cluster is sub-cluster-1, sub-cluster-2 and Blackhole is 45%, 45% and 10%.


# Instance level load balance

- Usually, a sub cluster is composed of multiple instances. Within sub cluster, WRR（weighted round robin) is used to distribute message among instance。
- Instance can be assign different weight based on its capacity。

# Health check of instance
BFE do health check for each backend instance. A instance has following two states: 

- NORMAL state：the instance act normally in processing message.
- CHECKING state：the instance is abnormal and can't process message. BFE do health check periodically in this state. 

State switch:
- NORMAL to CHECKING, when：
    - Consecutive failure, in connecting or sending message to the instance, exceed a threshhod.

- CHECKING to NORMAL, when：
    - BFE receive correct response from backend instance for health check request.


## Message retry in failure

If message routing fail, BFE support retry the message in two levels：

- Re-route message within same sub cluster.
- Re-route message to other sub cluster.

## Connection pool

TCP connection between BFE and backend instance support：

- short-lived connection：BFE route each request message to backend server with a new established TCP connection.

- connection pool：
    - BFE maintain connection pool to instances.
    - To an incoming request：
        - if there is an available connection, reuse it.
        - else establish a new TCP connection.

    - After finish processing a request via a connection:
        - If current size of connection pool is less than configured number, the connection is added into the pool.
        - Else close the connection directly.

## Session stickiness

BFE support session stickiness based on following identity of request message:
- Source IP
- Field in request header, cookie etc.

Keep session in different routing level:
- Sub cluster level: message of a session is sent to same sub cluster (may be different instance in this sub cluster).
- Instance level: message of a session is sent to same instance.

