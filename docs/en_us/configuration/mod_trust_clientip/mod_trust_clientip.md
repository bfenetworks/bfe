# Introduction 

Check client IP of incoming request against trusted ip dict. If matched, mark request/connection is trusted.

# Configuration

- Module config file

  conf/mod_trust_clientip/mod_trust_clientip.conf

  ```
  [basic]
  DataPath = ../conf/mod_trust_clientip/trust_client_ip.data
  ```

- Trusted IP data file

  conf/mod_trust_clientip/trust_client_ip.data

| Config Item | Type   | Description                                                     |
| ----------- | ------ | --------------------------------------------------------------- |
| Version     | String | Verson of config file                                           |
| Config      | Struct | trusted client ip dict. Key: lable, Value: a list of IP segment |

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

