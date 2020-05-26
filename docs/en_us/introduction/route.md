# Request Routing

## Overview

- In BFE [forwarding model](./forward_model.md), after "product name" for one request is determined, the destionation cluster should be identified.
- A Forwarding Table is provided for each product inside BFE.
- For each request, the forwarding table for the determined product is searched, and destination cluster is identified.

## Forwarding Table

- Forwarding table is composed by one or more "forwarding rules".
- Each forwarding rule has two parts: condition(for matching the request), destionation cluster.
- Condition is expressed in [Condition Expression](../condition).
- For a request, multiple rules in forwarding table are searched up to down.(i.e., the order of forwarding rules is very important.) The procedure will stop if some rule is matched.
- There must be one Default Rule in the forwarding table. If no other rules is matched for a request, the destionation cluster defined in Default Rule is returned.


## Example

- A product "demo" has three clusters:
    - demo-static：serve static content 
    - demo-post：serve post request
    - demo-main: serve other traffic

- The expected scenarios:
    - Requests with path prefixed with "/static" will be forwarded to cluster demo-static.
    - Request using method "POST" and prefixed with "/setting" will be forwarded to cluster "demo-post". 
    - Other request will be forwarded to cluster "demo-main".

- The corresponding forwarding table is shown as follows.

![Forwarding Table](../../images/forwarding_table.png)