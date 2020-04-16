# Request balancing

## Sub cluster level load balance

- In sub cluster level, load balance rules also can be configured. The rules define traffic weight which is distributed to each sub cluster。

- A special virtual sub cluster "BLACKHOLE" could be used to discard traffic。

### Example

- Consider following configuration：
    - Two IDC：IDC1, IDC2
    - Two BFE cluster：BFE1, BFE2
    - Two backend sub cluster：sub-cluster-1, sub-cluster-2

- In BFE cluster(BFE1 and BFE2):
    - BFE1：{sub-cluster-1:w11，sub-cluster-2:w12, Blackhole:w1B}
    - BFE2：{sub-cluster-1:w21，sub-cluster-2:w22, Blackhole:w2B}

- Based on the weight in configuration, BFE distribute traffic to backend sub cluster.
    - For example，if weight configuration {w11, w12, w1B} ={45，45，10}, percent of traffic to sub cluster is sub-cluster-1, sub-cluster-2 and Blackhole is 45%, 45% and 10%.


## Instance level load balance

- Usually, a sub cluster is composed of multiple instances. Within sub cluster, WRR（weighted round robin) is used to distribute request among instance。
- Instance can be assign different weight based on its capacity。

## Health check
BFE do health check for each backend instance. A instance has following two states: 

- NORMAL state：the instance act normally in processing request.
- CHECKING state：the instance is abnormal and can't process request. BFE do health check periodically in this state. 

State switch:
- NORMAL to CHECKING, when：
    - Consecutive failure, in connecting or sending request to the instance, exceed a threshhod.

- CHECKING to NORMAL, when：
    - BFE receive correct response from backend instance for health check request.


## Retry in failure

If request routing fail, BFE support retry the request in two levels：

- Re-route request within same sub cluster.
- Re-route request to other sub cluster.

## Connection pool

TCP connection between BFE and backend instance support：

- short-lived connection：BFE route each request request to backend server with a new established TCP connection.

- connection pool：
    - BFE maintain connection pool to instances.
    - To an incoming request：
        - if there is an available connection, reuse it.
        - else establish a new TCP connection.

    - After finish processing a request via a connection:
        - If current size of connection pool is less than configured number, the connection is added into the pool.
        - Else close the connection directly.

## Session stickiness

BFE support session stickiness based on following identity of request request:
- Source IP
- Field in request header, cookie etc.

Keep session in different routing level:
- Sub cluster level: request of a session is sent to same sub cluster (may be different instance in this sub cluster).
- Instance level: request of a session is sent to same instance.

