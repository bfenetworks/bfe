# Naming convention of condition primitives

BFE adopts the following naming convention for condition primitives.

## Name prefix of condition primitives

- Name prefix of the request primitive is "**req_**"
    - e.g. req_host_in()

- Name prefix of the response primitive is "**res_**"
    - e.g. res_code_in()

- Name prefix of the session primitive is "**ses_**"
    - e.g. ses_vip_in()

- Name prefix of the system primitive is "**bfe_**"
    - e.g. bfe_time_range()

## Name of comparison operations

- **match**: exact match
    - eg. req_tag_match()

- **in**: whether an element exists in a set or not
    - eg. req_host_in()

- **prefix_in**: whether the prefix exists in a set or not
    - eg. req_path_prefix_in()

- **suffix_in**: whether the suffix exists in a set or not
    - eg. req_path_suffix_in()

- **key_exist**: whether the specified key exists or not
    - eg. req_query_key_exist()

- **value_in**: whether the value exists in a set or not
    - eg. req_query_key_exist()

- **value_prefix_in**: whether the value prefix exists in a set or not
    - eg. req_header_value_prefix_in()

- **value_suffix_in**: whether the value suffix exists in a set or not
    - eg. req_header_value_suffix_in()

- **range**: range match
    - eg. req_cip_range()

- **regmatch**: use regular expression to match
    - eg. req_url_regmatch()
    - Warning:  Inappropriate use can significantly affect performance

- **contain**: string match
    - eg. req_cookie_value_contain()
