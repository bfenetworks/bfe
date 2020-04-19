# Introduction 

Check client IP of incoming request against trusted ip dict. If matched, mark request/connection is trusted.

# Module configuration

## Description
conf/mod_trust_clientip/mod_trust_clientip.conf

| Config Item | Description                             |
| ----------- | --------------------------------------- |
| Basic.DataPath | String<br>path of rule configuraiton |

## Example
```
[basic]
DataPath = mod_trust_clientip/trust_client_ip.data
```

# Rule configuraiton

## Description
  conf/mod_trust_clientip/trust_client_ip.data

| Config Item       | Type   | Description                                                     |
| ----------------- | ------ | --------------------------------------------------------------- |
| Version           | String | Verson of config file                                           |
| Config            | Object | trusted ip config |
| Config{k}         | Struct | label
| Config{v}         | String | A list of ip segments |
| Config{v}[]       | Object | A ip segment |
| Config{v}[].Begin | String | start ip address |
| Config{v}[].End   | String | end ip address |

## Example
```
{
    "Version": "20190101000000",
    "Config": {
        "inner-idc": [
            {
                "Begin": "10.0.0.0",
                "End": "10.255.255.255"
            }
        ]
    }
}
```

