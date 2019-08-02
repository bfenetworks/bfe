# Block

## Scenario

* Suppose our service is attacked by a specific IP, or a specific interface (such as issuing a voucher) is maliciously called;
* We hope to simply block traffic
  * block IP: Attack traffic comes from some fixed IP(2.2.2.2)
  * block PATH: Attack traffic for specific PATH(/bonus)

Add some configuration on the [basic routing configuration](route.md), you can implement block.

See the [complete configuration](../../../example_conf/block) for details

  * Step 1. Enable mod_block
      * See the complete configuration [bfe.conf](../../../example_conf/block/bfe.conf) for details

```
+++ Modules = mod_block  (add a line to bfe.conf, enable mod_block)
```

* Step 2. Configure mod_block
  * configure data path of mod_block, including file path of global ip blacklist and block rules
  * configure [mod_block.conf](../../../example_conf/block/mod_block/mod_block.conf) as below

```
[basic]
# file path of product rules
ProductRulePath = mod_block/block_rules.data

# file path of global ip blacklist
IPBlacklistPath = mod_block/ip_blacklist.data
```

* Step 3. Configure block rules
  * block IP 2.2.2.2, configure [ip_blacklist.data](../../../example_conf/block/mod_block/ip_blacklist.data) as below
  
  ```
  2.2.2.2
  ```
  
  * block PATH /bonus, configure [block_rules.data](../../../example_conf/block/mod_block/block_rules.data) as below
  
  ```
  {
      "Version": "init version",
      "Config": {
          "example_product": [{
              "action": {
                  "cmd": "CLOSE",
                  "params": []
              },
              "name": "block bonus",
              "cond": "req_path_in(\"/bonus\", false)"
          }]
      }
  }
  ```


* Now, use curl to verify whether it can be blocked successfully.

curl -v -H "host: example.org" "http://127.1:8080/bonus" will close connection directly.
