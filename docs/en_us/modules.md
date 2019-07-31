# Plugin module

## rewrite

### Functionality

- Modify URI of HTTP request based on defined rule

### Configuration

- Rule include：
    - condition："condition" expression defined how to match message. 
    - action: what to do after matched
    - stop after match：true

## redirect

### Functionality

- Redirect HTTP request based on defined rule

### Configuration

- Rule include：
    - condition："condition" expression defined how to match message
    - action：what to do after matched
    - status：http status code

## header

### Functionality

- Modify header of HTTP request or response

### Configuration

- Module config file

    conf/mod_header/mod_header.conf

    ```
    [basic]

    DataPath = ../conf/mod_header/header_rule.data
    ```

- Rule config file

    conf/mod_header/header_rule.data

## block

### Functionality

- IP Blacklist, such as block a request from some subnet.

- Block message which matches "condition" expression

- Close connection if message is blocked.

### Configuration

- Module config file

    conf/mod_block/mod_block.conf
    ```
    [basic]

    # product rule config file path

    ProductRulePath = ../conf/mod_block/block_rules.data

    # global ip blacklist file path

    IPBlacklistPath = ../conf/mod_block/ip_blacklist.data
    ```
- Data config file

    - blacklist file

        conf/mod_block/ip_blacklist.data

    - block rule file

        conf/mod_block/block_rules.data

## logid

### Functionality

- In forwarded request, a new http header "Bfe_logid" is added to identify the reqeust. This logid is unique in the whole BFE cluster.

- Generating algorithm
    - hashing with following parameters:
        - protocol type: tcp or udp
        - source address: ip:port
        - destination address: ip:port
        - BFE process id
        - time

## trust_clientip

### Functionality

Set a trusted IP list, and mark incoming request if its source IP is included in the list.

### Configuration

- Module config file

    conf/mod_trust_clientip/mod_trust_clientip.conf

    ```
    [basic]

    DataPath = ../conf/mod_trust_clientip/trust_client_ip.data
    ```

- Trusted IP data file

    conf/mod_trust_clientip/trust_client_ip.data

