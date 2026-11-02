// Copyright (c) 2025 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/* legacy_patcher.go - rewrite legacy primitive names to Env methods via expr Patcher
/*
DESCRIPTION
	expr.Patch 在类型检查前作用于 AST：把旧原语调用的函数名重写为 Env 方法名，
	重写后的名字参与 Env 绑定和编译期类型检查。
	未识别的名字原样保留，由类型检查阶段报 unknown name（错误信息保留用户原文）。

	移植裁剪说明（目标仓库 bfe 不支持的原语，一律不映射，命中会以未知函数报错）：
	- ses_tls_client_auth：bfe_tls.ConnectionState 缺少 ClientAuthType/VerifiedChainsGM
	- req_vport_in、req_cluster_in、req_sub_cluster_in、bfe_cluster_in、req_error_in、
	  usr_variable_is_empty、usr_variable_value_in、apikey_in、model_in：
	  源仓库扩展原语，bfe 旧引擎清单中不存在
*/

package bfe_expr

import (
	"bytes"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/bfenetworks/bfe/bfe_util"
	"github.com/bfenetworks/bfe/bfe_util/net_util"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// legacyNameMap 旧原语名 -> Env 方法名。
// 清单照 parser/scanner.go 的 funcProtos（旧原语权威列表），是本兼容层的能力清单。
//
// 注意：time_weekday_in、req_body_json_prefix_in、req_body_larger_than、
// req_body_less_than、req_ai_intent_in 在目标仓库旧原语清单中存在，
// 本期（条件引擎一期）不提供实现，有意不加入本表。
var legacyNameMap = map[string]string{
	"default_t":                  "DefaultT",
	"req_cip_trusted":            "ReqCipTrusted",
	"req_vip_in":                 "ReqVipIn",
	"req_proto_match":            "ReqProtoMatch",
	"req_proto_secure":           "ReqProtoSecure",
	"req_host_in":                "ReqHostIn",
	"req_host_regmatch":          "ReqHostRegmatch",
	"req_host_tag_in":            "ReqHostTagIn",
	"req_host_suffix_in":         "ReqHostSuffixIn",
	"req_path_in":                "ReqPathIn",
	"req_path_prefix_in":         "ReqPathPrefixIn",
	"req_path_suffix_in":         "ReqPathSuffixIn",
	"req_path_contain":           "ReqPathContain",
	"req_path_regmatch":          "ReqPathRegmatch",
	"req_path_element_prefix_in": "ReqPathElementPrefixIn",
	"req_query_key_prefix_in":    "ReqQueryKeyPrefixIn",
	"req_query_key_in":           "ReqQueryKeyIn",
	"req_query_exist":            "ReqQueryExist",
	"req_query_value_in":         "ReqQueryValueIn",
	"req_query_value_prefix_in":  "ReqQueryValuePrefixIn",
	"req_query_value_suffix_in":  "ReqQueryValueSuffixIn",
	"req_query_value_regmatch":   "ReqQueryValueRegmatch",
	"req_query_value_hash_in":    "ReqQueryValueHashIn",
	"req_query_value_contain":    "ReqQueryValueContain",
	"req_url_regmatch":           "ReqUrlRegmatch",
	"req_cookie_key_in":          "ReqCookieKeyIn",
	"req_cookie_value_in":        "ReqCookieValueIn",
	"req_cookie_value_prefix_in": "ReqCookieValuePrefixIn",
	"req_cookie_value_suffix_in": "ReqCookieValueSuffixIn",
	"req_cookie_value_hash_in":   "ReqCookieValueHashIn",
	"req_cookie_value_contain":   "ReqCookieValueContain",
	"req_port_in":                "ReqPortIn",
	"req_tag_match":              "ReqTagMatch",
	"req_ua_regmatch":            "ReqUaRegmatch",
	"req_header_key_in":          "ReqHeaderKeyIn",
	"req_header_value_in":        "ReqHeaderValueIn",
	"req_header_value_prefix_in": "ReqHeaderValuePrefixIn",
	"req_header_value_suffix_in": "ReqHeaderValueSuffixIn",
	"req_header_value_regmatch":  "ReqHeaderValueRegmatch",
	"req_header_value_hash_in":   "ReqHeaderValueHashIn",
	"req_header_value_contain":   "ReqHeaderValueContain",
	"req_method_in":              "ReqMethodIn",
	"req_cip_range":              "ReqCipRange",
	"req_cip_hash_in":            "ReqCipHashIn",
	"req_vip_range":              "ReqVipRange",
	"res_code_in":                "ResCodeIn",
	"res_header_key_in":          "ResHeaderKeyIn",
	"res_header_value_in":        "ResHeaderValueIn",
	"ses_vip_range":              "SesVipRange",
	"ses_sip_range":              "SesSipRange",
	"ses_tls_sni_in":             "SesTlsSniIn",
	"ses_tls_client_ca_in":       "SesTlsClientCaIn",
	"req_context_value_in":       "ReqContextValueIn",
	"bfe_time_range":             "BfeTimeRange",
	"bfe_periodic_time_range":    "BfePeriodicTimeRange",
	"req_body_contain":           "ReqBodyContain",
	"req_body_json_in":           "ReqBodyJsonIn",
}

// legacyPatcher 重写 CallNode 的被调函数名（仅命中 legacyNameMap 的标识符）
type legacyPatcher struct{}

func (p *legacyPatcher) Visit(node *ast.Node) {
	call, ok := (*node).(*ast.CallNode)
	if !ok {
		return
	}
	id, ok := call.Callee.(*ast.IdentifierNode)
	if !ok {
		return
	}
	if repl, ok := legacyNameMap[id.Value]; ok {
		id.Value = repl
	}
}

// ---------- 旧原语常量参数校验（对齐旧实现的加载期检查） ----------

// legacyArgValidators 旧原语名 -> 常量参数校验函数；仅校验字面量参数
var legacyArgValidators = map[string]func(call *ast.CallNode) error{
	"req_host_regmatch":         validateRegexArg(0),
	"req_path_regmatch":         validateRegexArg(0),
	"req_url_regmatch":          validateRegexArg(0),
	"req_ua_regmatch":           validateRegexArg(0),
	"req_query_value_regmatch":  validateRegexArg(1),
	"req_header_value_regmatch": validateRegexArg(1),

	"req_cip_hash_in":          validateHashSectionsArg(0),
	"req_query_value_hash_in":  validateHashSectionsArg(1),
	"req_cookie_value_hash_in": validateHashSectionsArg(1),
	"req_header_value_hash_in": validateHashSectionsArg(1),

	"bfe_time_range":          validateTimeRangeArgs,
	"bfe_periodic_time_range": validatePeriodicTimeArgs,

	"req_vip_in": validateVipInArg(0),

	"req_cip_range": validateIPRangeArgs,
	"req_vip_range": validateIPRangeArgs,
	"ses_vip_range": validateIPRangeArgs,
	"ses_sip_range": validateIPRangeArgs,
}

func stringArg(call *ast.CallNode, i int) (string, bool) {
	if i >= len(call.Arguments) {
		return "", false
	}
	s, ok := call.Arguments[i].(*ast.StringNode)
	return s.Value, ok
}

// validateRegexArg 对齐旧 build：正则在加载期编译
func validateRegexArg(i int) func(*ast.CallNode) error {
	return func(call *ast.CallNode) error {
		pattern, ok := stringArg(call, i)
		if !ok {
			return nil // 非常量参数跳过（运行期 regexMatch 错误按 false 处理）
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("primitive %s: compile regexp %q: %v",
				call.Callee.String(), pattern, err)
		}
		return nil
	}
}

// validateHashSectionsArg 对齐旧 NewHashMatcher：桶区间在加载期解析
func validateHashSectionsArg(i int) func(*ast.CallNode) error {
	return func(call *ast.CallNode) error {
		sections, ok := stringArg(call, i)
		if !ok {
			return nil
		}
		for _, section := range strings.Split(sections, "|") {
			if _, _, ok := parseHashSection(section); !ok {
				return fmt.Errorf("primitive %s: invalid hash section %q",
					call.Callee.String(), section)
			}
		}
		return nil
	}
}

// validateTimeRangeArgs 对齐旧 NewTimeMatcher：时间格式与 start<=end 在加载期检查
func validateTimeRangeArgs(call *ast.CallNode) error {
	start, ok1 := stringArg(call, 0)
	end, ok2 := stringArg(call, 1)
	if !ok1 || !ok2 {
		return nil
	}
	startTime, err := bfe_util.ParseTime(start)
	if err != nil {
		return fmt.Errorf("primitive %s: startTime %q invalid: %v", call.Callee.String(), start, err)
	}
	endTime, err := bfe_util.ParseTime(end)
	if err != nil {
		return fmt.Errorf("primitive %s: endTime %q invalid: %v", call.Callee.String(), end, err)
	}
	if startTime.After(endTime) {
		return fmt.Errorf("primitive %s: startTime %q must <= endTime %q", call.Callee.String(), start, end)
	}
	return nil
}

// validatePeriodicTimeArgs 对齐旧 NewPeriodicTimeMatcher
func validatePeriodicTimeArgs(call *ast.CallNode) error {
	start, ok1 := stringArg(call, 0)
	end, ok2 := stringArg(call, 1)
	period, _ := stringArg(call, 2)
	if !ok1 || !ok2 {
		return nil
	}
	if period != "" {
		return fmt.Errorf("primitive %s: periodStr %q is not supported", call.Callee.String(), period)
	}
	_, offset1, err := bfe_util.ParseTimeOfDay(start)
	if err != nil {
		return fmt.Errorf("primitive %s: startTime %q invalid: %v", call.Callee.String(), start, err)
	}
	_, offset2, err := bfe_util.ParseTimeOfDay(end)
	if err != nil {
		return fmt.Errorf("primitive %s: endTime %q invalid: %v", call.Callee.String(), end, err)
	}
	if offset1 != offset2 {
		return fmt.Errorf("primitive %s: timezone of start and end should be same", call.Callee.String())
	}
	return nil
}

// validateVipInArg 对齐旧 NewIpInMatcher：列表元素须全部为合法 IP（plain split，无 CIDR）
func validateVipInArg(i int) func(*ast.CallNode) error {
	return func(call *ast.CallNode) error {
		patterns, ok := stringArg(call, i)
		if !ok {
			return nil
		}
		for _, ipStr := range strings.Split(patterns, "|") {
			if net.ParseIP(ipStr) == nil {
				return fmt.Errorf("primitive %s: invalid IP addr string:%s",
					call.Callee.String(), ipStr)
			}
		}
		return nil
	}
}

// validateIPRangeArgs 对齐旧 NewIPMatcher：两端为合法 IP、v4/v6 一致、start<=end
func validateIPRangeArgs(call *ast.CallNode) error {
	start, ok1 := stringArg(call, 0)
	end, ok2 := stringArg(call, 1)
	if !ok1 || !ok2 {
		return nil
	}
	if net.ParseIP(start) == nil {
		return fmt.Errorf("primitive %s: invalid IP addr string:%s", call.Callee.String(), start)
	}
	if net.ParseIP(end) == nil {
		return fmt.Errorf("primitive %s: invalid IP addr string:%s", call.Callee.String(), end)
	}
	if net_util.IsIPv4Address(start) != net_util.IsIPv4Address(end) {
		return fmt.Errorf("primitive %s: startIP[%s] and endIP[%s] has different addr type(IPv4/IPv6)",
			call.Callee.String(), start, end)
	}
	if bytes.Compare(net.ParseIP(end).To16(), net.ParseIP(start).To16()) < 0 {
		return fmt.Errorf("primitive %s: startIP[%s] must <= endIP[%s]", call.Callee.String(), start, end)
	}
	return nil
}

// legacyArgCheckVisitor 收集并校验旧原语调用的常量参数
type legacyArgCheckVisitor struct {
	err error
}

func (v *legacyArgCheckVisitor) Visit(node *ast.Node) {
	if v.err != nil {
		return
	}
	call, ok := (*node).(*ast.CallNode)
	if !ok {
		return
	}
	id, ok := call.Callee.(*ast.IdentifierNode)
	if !ok {
		return
	}
	validate, ok := legacyArgValidators[id.Value]
	if !ok {
		return
	}
	v.err = validate(call)
}

// ValidateLegacyArgs 校验条件串中旧原语字面量参数（正则/哈希区间/时间格式），
// 使旧实现"加载期拒绝非法配置"的行为在新引擎下保持等价
func ValidateLegacyArgs(src string) error {
	tree, err := parser.Parse(src)
	if err != nil {
		return err
	}
	v := &legacyArgCheckVisitor{}
	node := tree.Node
	ast.Walk(&node, v)
	return v.err
}
