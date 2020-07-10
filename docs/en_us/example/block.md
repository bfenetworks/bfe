# Block

## Scenario

* Suppose our service has been attacked from a specific IP, or a specific API (such as issuing a voucher) has been maliciously called; we want to block specified traffic, such as:
  * block attack traffic comes from some fixed IP（2.2.2.2）
  * block attack traffic targeted at some specific PATH（/bonus）

## Configuration

Modify example configurations (conf/) as the following steps:

* Step 1. Modify conf/bfe.conf and enable mod_block

```ini
Modules = mod_block   #enable mod_block
```

* Step 2. Modify conf/mod_block/mod_block.conf and configure path of global ip blocklist and block rules
  
```ini
[basic]
ProductRulePath = mod_block/block_rules.data

IPBlocklistPath = mod_block/ip_blocklist.data
```
  
* Step 3. Configure global blocklist (conf/mod_block/ip_blocklist.data)
  
Config ip address list, such as 2.2.2.2
  
```ini
2.2.2.2
```

* Step 4. Configure block rules (conf/mod_block/block_rules.data)
  
```json
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
  
* Step 5. Verify configured rules

```bash
curl -v -H "host: example.org" "http://127.1:8080/bonus"
```

The connection will be closed by bfe immediately.
