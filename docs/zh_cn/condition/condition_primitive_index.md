# 条件原语索引

## 请求相关
 * req_cip_hash_in(value_list)
 * req_cip_range(start_ip, end_ip)
 * req_cip_trusted()
 * req_cookie_key_in(key_list)
 * req_cookie_value_contain(key, value, case_insensitive)
 * req_cookie_value_in(key, value_list, case_insensitive)
 * req_cookie_value_hash_in(key, value_list, case_insensitive)
 * req_cookie_value_prefix_in(key, value_prefix_list, case_insensitive)
 * req_cookie_value_suffix_in(key, value_suffix_list, case_insensitive)
 * req_header_key_in(key_list)
 * req_header_value_contain(key, value_list, case_insensitive)
 * req_header_value_in(header_name, value_list, case_insensitive)
 * req_header_value_hash_in(header_name, value_list, case_insensitive)
 * req_header_value_prefix_in(header_name, value_prefix_list, case_insensitive)
 * req_header_value_suffix_in(header_name, value_suffix_list, case_insensitive)
 * req_host_in(host_list)
 * req_method_in(method_list)
 * req_proto_secure()
 * req_tag_match(tagName, tagValue)
 * req_path_in(path_list, case_insensitive)
 * req_path_prefix_in(prefix_list, case_insensitive)
 * req_path_suffix_in(suffix_list, case_insensitive)
 * req_path_element_suffix_in(suffix_list, case_insensitive)
 * req_query_key_in(key_list)
 * req_query_key_prefix_in(prefix_list)
 * req_query_value_in(key,  value_list, case_insensitive)
 * req_query_value_hash_in(key, value_list, case_insensitive)
 * req_query_value_prefix_in(key, prefix_list, case_insensitive)
 * req_query_value_suffix_in(key, suffix_list, case_insensitive)
 * req_port_in(port_list)
 * req_url_regmatch(reg_exp)
 * req_vip_in(vip_list)
 * req_vip_range(start_ip, end_ip)

## 响应相关
 * res_code_in(codes)
 * res_header_key_in(key_list)
 * res_header_value_in(key, value_list, case_insensitive)

## 会话相关
 * ses_sip_range(start_ip, end_ip)
 * ses_vip_range(start_ip, end_ip)
 * ses_tls_sni_in(host_list)
 * ses_tls_client_auth()
 * ses_tls_client_ca_in(ca_list)

## 系统相关
 * bfe_time_range(start_time, end_time)

