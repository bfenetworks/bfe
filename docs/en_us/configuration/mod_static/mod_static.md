# Introduction 

mod_static serves static files.

# Configuration

- Module config file

  conf/mod_static/mod_static.conf

  ```
  [basic]
  DataPath = ../conf/mod_static/static_rule.data
  ```

- Rule config file

  conf/mod_static/static_rule.data

| Action                    | Description                        |
| ------------------------- | ---------------------------------- |
| BROWSE                    | Serve static files. <br>The first parameter is the location of root directory.<br> The second parameter is the name of default file.|

   ```
    {
        "Config": {
            "example_product": [
                {
                    "Cond": "req_host_in(\"www.example.org\")",
                    "Action": {
                        "Cmd": "BROWSE",
                        "Params": [
                            "./",
                            "index.html"
                        ]
                    }
                }
            ]
        },
        "Version": "20190101000000"
    }
  ```
