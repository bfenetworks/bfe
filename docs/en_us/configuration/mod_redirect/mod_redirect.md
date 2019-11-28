# Introduction 

Redirect HTTP request based on defined rules.

# Configuration

- Module config file

  conf/mod_redirect/mod_redirect.conf

  ```
  [basic]
  DataPath = ../conf/mod_redirect/redirect.data
  ```

- Rule config file

  conf/mod_redirect/redirect.data

| Config Item | Type   | Description                                                  |
| ----------- | ------ | ------------------------------------------------------------ |
| Version     | String | Verson of config file                                        |
| Config      | Struct | Redirect rules for each product. Redirect rule include: <br>- Cond: "condition" expression <br>- Actions: what to do after matched<br>- Status: response status code |

| Action         | Description                                                                         |
| -------------- | ----------------------------------------------------------------------------------- |
| URL_SET        | redirect to specified URL                                                           |
| URL_FROM_QUERY | redirect to URL parsed from specified query in request                              |
| URL_PREFIX_ADD | redirect to URL concatenated by specified prefix and the original URL               |
| SCHEME_SET     | redirect to the original URL but with scheme changed. supported scheme: http\|https |
  
  ```
  {
      "Version": "20190101000000",
      "Config": {
          "example_product": [
              {
                  "Cond": "req_path_prefix_in(\"/redirect\", false)",
                  "Actions": [
                      {
                          "Cmd": "URL_SET",
                          "Params": ["https://example.org"]
                      }
                  ],
                  "Status": 301
              }
          ]
      }
  }
  ```
  
  
