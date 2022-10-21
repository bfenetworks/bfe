# 条件原语索引

## 请求相关

### cip

 * [req_cip_hash_in(value_list)](./request/ip.md#req_cip_hash_invalue_list)
 * [req_cip_range(start_ip, end_ip)](./request/ip.md#req_cip_rangestart_ip-end_ip)
 * [req_cip_trusted()](./request/ip.md#req_cip_trusted)

### context

 * [req_context_value_in(key, value_list, case_insensitive)](./request/context.md#req_context_value_inkey-value_list-case_insensitive)

### cookie

 * [req_cookie_key_in(key_list)](./request/cookie.md#req_cookie_key_inkey_list)
 * [req_cookie_value_contain(key, value, case_insensitive)](./request/cookie.md#req_cookie_value_containkey-value-case_insensitive)
 * [req_cookie_value_hash_in(key, value_list, case_insensitive)](./request/cookie.md#req_cookie_value_hash_inkey-value_list-case_insensitive)
 * [req_cookie_value_in(key, value_list, case_insensitive)](./request/cookie.md#req_cookie_value_inkey-value_list-case_insensitive)
 * [req_cookie_value_prefix_in(key, value_prefix_list, case_insensitive)](./request/cookie.md#req_cookie_value_prefix_inkey-value_prefix_list-case_insensitive)
 * [req_cookie_value_suffix_in(key, value_suffix_list, case_insensitive)](./request/cookie.md#req_cookie_value_suffix_inkey-value_suffix_list-case_insensitive)

### header

 * [req_header_key_in(key_list)](./request/header.md#req_header_key_inkey_list)
 * [req_header_value_contain(key, value_list, case_insensitive)](./request/header.md#req_header_value_containheader_name-value_list-case_insensitive)
 * [req_header_value_hash_in(header_name, value_list, case_insensitive)](./request/header.md#req_header_value_hash_inheader_name-value_list-case_insensitive)
 * [req_header_value_in(header_name, value_list, case_insensitive)](./request/header.md#req_header_value_inheader_name-value_list-case_insensitive)
 * [req_header_value_prefix_in(header_name, value_prefix_list, case_insensitive)](./request/header.md#req_header_value_prefix_inheader_name-value_prefix_list-case_insensitive)
 * [req_header_value_suffix_in(header_name, value_suffix_list, case_insensitive)](./request/header.md#req_header_value_suffix_inheader_name-value_suffix_list-case_insensitive)

### host

 * [req_host_in(host_list)](./request/uri.md#req_host_inhost_list)

### method

 * [req_method_in(method_list)](./request/method.md#req_method_inmethod_list)

### path

 * [req_path_contain(path_list, case_insensitive)](./request/uri.md#req_path_containpath_list-case_insensitive)
 * [req_path_element_prefix_in(prefix_list, case_insensitive)](./request/uri.md#req_path_element_prefix_inprefix_list-case_insensitive)
 * [req_path_in(path_list, case_insensitive)](./request/uri.md#req_path_inpath_list-case_insensitive)
 * [req_path_prefix_in(prefix_list, case_insensitive)](./request/uri.md#req_path_prefix_inprefix_list-case_insensitive)
 * [req_path_suffix_in(suffix_list, case_insensitive)](./request/uri.md#req_path_suffix_insuffix_list-case_insensitive)

### port

 * [req_port_in(port_list)](./request/uri.md#req_port_inport_list)

### protocol

 * [req_proto_secure()](./request/protocol.md#req_proto_secure)

### query

 * [req_query_key_in(key_list)](./request/uri.md#req_query_key_inkey_list)
 * [req_query_key_prefix_in(prefix_list)](./request/uri.md#req_query_key_prefix_inprefix_list)
 * [req_query_value_hash_in(key, value_list, case_insensitive)](./request/uri.md#req_query_value_hash_inkey-value_list-case_insensitive)
 * [req_query_value_in(key,  value_list, case_insensitive)](./request/uri.md#req_query_value_inkey-value_list-case_insensitive)
 * [req_query_value_prefix_in(key, prefix_list, case_insensitive)](./request/uri.md#req_query_value_prefix_inkey-prefix_list-case_insensitive)
 * [req_query_value_suffix_in(key, suffix_list, case_insensitive)](./request/uri.md#req_query_value_suffix_inkey-suffix_list-case_insensitive)

### tag

 * [req_tag_match(tagName, tagValue)](./request/tag.md#req_tag_matchtagname-tagvalue)

### url

 * [req_url_regmatch(reg_exp)](./request/uri.md#req_url_regmatchreg_exp)

### vip

 * [req_vip_in(vip_list)](./request/ip.md#req_vip_invip_list)
 * [req_vip_range(start_ip, end_ip)](./request/ip.md#req_vip_rangestart_ip-end_ip)

## 响应相关

### code

 * [res_code_in(codes)](./response/code.md#res_code_incodes)

### header

 * [res_header_key_in(key_list)](./response/header.md#res_header_key_inkey_list)
 * [res_header_value_in(key, value_list, case_insensitive)](./response/header.md#res_header_value_inkey-value_list-case_insensitive)

## 会话相关

### sip

 * [ses_sip_range(start_ip, end_ip)](./session/ip.md#ses_sip_rangestart_ip-end_ip)

### tls client

 * [ses_tls_client_auth()](./session/tls.md#ses_tls_client_auth)
 * [ses_tls_client_ca_in(ca_list)](./session/tls.md#ses_tls_client_ca_inca_list)

### tls sni

 * [ses_tls_sni_in(host_list)](./session/tls.md#ses_tls_sni_inhost_list)

### vip

 * [ses_vip_range(start_ip, end_ip)](./session/ip.md#ses_vip_rangestart_ip-end_ip)

## 系统相关

### time

 * [bfe_periodic_time_range(start_time, end_time, period)](./system/time.md#bfe_periodic_time_rangestart_time-end_time-period)
 * [bfe_time_range(start_time, end_time)](./system/time.md#bfe_time_rangestart_time-end_time)
