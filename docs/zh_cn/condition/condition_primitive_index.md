# 条件原语索引

## 请求相关
 * req_cip_hash_in(patterns)
 * req_cip_range(start_ip, end_ip)
 * req_cip_trusted()
 * req_cookie_key_in(patterns)
 * req_cookie_value_contain(key, patterns, case_insensitive)
 * req_cookie_value_in(key, patterns, case_insensitive)
 * req_cookie_value_hash_in(key, patterns, case_insensitive)
 * req_cookie_value_prefix_in(key, patterns, case_insensitive)
 * req_cookie_value_suffix_in(key, patterns, case_insensitive)
 * req_header_key_in(patterns)
 * req_header_value_contain(key, patterns, case_insensitive)
 * req_header_value_in(header_name, patterns, case_insensitive)
 * req_header_value_hash_in(header_name, patterns, case_insensitive)
 * req_header_value_prefix_in(header_name, patterns, case_insensitive)
 * req_header_value_suffix_in(header_name, patterns, case_insensitive)
 * req_host_in(patterns)
 * req_method_in(patterns)
 * req_proto_secure()
 * req_tag_match(tag_name, tag_value)
 * req_path_in(patterns, case_insensitive)
 * req_path_prefix_in(patterns, case_insensitive)
 * req_path_suffix_in(patterns, case_insensitive)
 * req_query_key_in(patterns)
 * req_query_key_prefix_in(patterns)
 * req_query_value_in(key, patterns, case_insensitive)
 * req_query_value_hash_in(key, patterns, case_insensitive)
 * req_query_value_prefix_in(key, patterns, case_insensitive)
 * req_query_value_suffix_in(key, patterns, case_insensitive)
 * req_port_in(patterns)
 * req_url_regmatch(patterns)
 * req_vip_in(patterns)
 * req_vip_range(start_ip, end_ip)

## 响应相关
 * res_code_in(codes)
 * res_header_key_in(patterns)
 * res_header_value_in(key, patterns, case_insensitive)

## 会话相关
 * ses_sip_range(start_ip, end_ip)
 * ses_vip_range(start_ip, end_ip)
 * ses_tls_sni_in(patterns)
 * ses_tls_client_auth()
 * ses_tls_client_ca_in(patterns)

## 系统相关
 * bfe_time_range(start_time, end_time)

