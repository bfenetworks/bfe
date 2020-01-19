# Condition Naming Convention

- Name prefix of condition primitive:
  - Condition primitive about request is used ”**req_**“ prefix
    - e.g. **req_host_in()**
  - Condition primitive about response is used ”**res_**“ prefix
    - e.g. **res_code_in()**
  - Condition primitive about session is used ”**ses_**“ prefix
    - e.g. **ses_vip_in()**
  - Condition primitive about system is used ”**bfe_**“ prefix
    - e.g. **bfe_time_range**()


- Name of compare actions:
  - **match**：exact match
    - In this situation, the only one parameter is given
    - eg. **req_tag_match()**
  - **in**：if the value is in the configured set
    - eg. **req_host_in()**
  - **prefix_in**：if the value prefix is in the configured set
    - eg. **req_path_prefix_in()**
  - **suffix_in**：if the value suffix is in the configured set
    - eg. **req_path_suffix_in()**
  - **key_exist**：if the key exists
    - eg. **req_query_key_exist()**
  - **value_in**：for the configured key, judge if the value is in the configured value set
    - eg. **req_query_key_exist()**
  - **value_prefix_in**：for the configured key, judge if the value prefix is in the configured set
    - eg. **req_header_value_prefix_in()**
  - **value_suffix_in**：for the configured key, judge if the value suffix is in the configured set
    - eg. **req_header_value_suffix_in()**
  - **range**: range match
    - eg. **req_cip_range()**
  - **regmatch**：regular match
    - eg. **req_url_regmatch()**
  - **contain**: string match
    - eg. **req_cookie_value_contain()**

