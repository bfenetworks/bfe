// Copyright (c) 2019 The BFE Authors.
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

// build condition by config ast

package condition

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition/parser"
	"github.com/bfenetworks/bfe/bfe_expr"
)

// EngineMode 条件引擎选择模式（进程级，由 bfe_server 依据 ConfigBasic.ConditionEngineMode 设置）
type EngineMode int

const (
	// EngineLegacyFirst 旧引擎优先（默认）：当成旧条件表达式处理，走旧引擎；
	// 仅当旧引擎编译失败时才用新引擎处理（如混写新函数、用户变量等）
	EngineLegacyFirst EngineMode = iota
	// EngineExprCompatible 新引擎 + 旧原语兼容：expr 编译，legacyPatcher 支持
	// 旧原语混用；编译失败不回退，直接报错
	EngineExprCompatible
	// EngineExprOnly 纯新表达式：expr 编译，不支持旧原语（旧原语名即未知函数），
	// 编译失败不回退，直接报错
	EngineExprOnly
)

func (m EngineMode) String() string {
	switch m {
	case EngineLegacyFirst:
		return "LegacyFirst"
	case EngineExprCompatible:
		return "ExprCompatible"
	case EngineExprOnly:
		return "ExprOnly"
	}
	return "Unknown"
}

// engineMode 进程级引擎模式，默认旧引擎优先
var engineMode = EngineLegacyFirst

// SetEngineMode 设置条件引擎模式。必须在任何 condition.Build 调用之前执行
// （bfe_server 在 InitBeforeStartUp 起始处调用，先于路由表与模块配置加载）。
func SetEngineMode(mode EngineMode) {
	engineMode = mode
}

// EngineModeString 返回当前引擎模式字符串（日志用）
func EngineModeString() string {
	return engineMode.String()
}

// Build builds condition by condition string.
//
// 双轨支持，引擎选择由 SetEngineMode 决定：
//  1. LegacyFirst（默认）：旧引擎优先，语义与旧版完全一致；旧引擎编译失败
//     （如混写新函数、用户变量等）回退新引擎；双失败 → 配置报错。
//  2. ExprCompatible：新引擎 + 旧原语兼容（legacyPatcher 重写旧原语名），
//     编译失败不回退，直接报错。
//  3. ExprOnly：纯新表达式，不支持旧原语，编译失败不回退，直接报错。
//
// 注意：两引擎对字符串字面量的转义处理不同（旧 scanner 只校验不解码、白名单含
// `\.`；expr 按 Go 规则解码），存在反斜杠转义的条件串在 LegacyFirst 与
// ExprCompatible/ExprOnly 下语义可能不同，这正是默认 LegacyFirst 的原因。
func Build(condStr string) (Condition, error) {
	switch engineMode {
	case EngineLegacyFirst:
		cond, err := legacyBuild(condStr)
		if err == nil {
			return cond, nil
		}
		exprCond, exprErr := bfe_expr.CompileLegacy(condStr)
		if exprErr == nil {
			return exprCond, nil
		}
		return nil, fmt.Errorf("condition %q: legacy build error: %v; expr compile error: %v",
			condStr, err, exprErr)

	case EngineExprOnly:
		return bfe_expr.Compile(condStr)

	default: // EngineExprCompatible
		return bfe_expr.CompileLegacy(condStr)
	}
}

// legacyBuild 旧引擎编译：parser.Parse + 变量检查 + build
func legacyBuild(condStr string) (Condition, error) {
	node, identList, err := parser.Parse(condStr)
	if err != nil {
		return nil, err
	}
	if len(identList) != 0 {
		return nil, fmt.Errorf("found unresolved variable %s %d", identList[0].Name, identList[0].Pos())
	}
	return build(node)
}

func build(node parser.Node) (Condition, error) {
	switch n := node.(type) {
	case *parser.CallExpr:
		return buildPrimitive(n)
	case *parser.UnaryExpr:
		return buildUnary(n)
	case *parser.BinaryExpr:
		return buildBinary(n)
	case *parser.ParenExpr:
		return build(n.X)
	default:
		return nil, fmt.Errorf("unsupported node %s", node)
	}
}

func buildUnary(node *parser.UnaryExpr) (Condition, error) {
	c, err := build(node.X)
	if err != nil {
		return nil, err
	}

	return &UnaryCond{op: node.Op, cond: c}, nil

}

func buildBinary(node *parser.BinaryExpr) (Condition, error) {
	l, err := build(node.X)
	if err != nil {
		return nil, err
	}

	r, err := build(node.Y)
	if err != nil {
		return nil, err
	}

	return &BinaryCond{op: node.Op, lc: l, rc: r}, nil
}

// buildPrimitive builds primitive from PrimitiveCondExpr.
// if failed, b.err is set to err, return Condition is nil
// if success, b.err is nil
func buildPrimitive(node *parser.CallExpr) (Condition, error) {
	switch node.Fun.Name {
	case "default_t":
		return &DefaultTrueCond{}, nil
	case "req_cip_trusted":
		return &TrustedCIpMatcher{}, nil
	case "req_vip_in":
		matcher, err := NewIpInMatcher(node.Args[0].Value)
		if err != nil {
			return nil, err
		}
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &VIPFetcher{},
			matcher: matcher,
		}, nil
	case "req_vip_range":
		matcher, err := NewIPMatcher(node.Args[0].Value, node.Args[1].Value)
		if err != nil {
			return nil, err
		}

		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &VIPFetcher{},
			matcher: matcher,
		}, nil
	case "req_cip_range":
		matcher, err := NewIPMatcher(node.Args[0].Value, node.Args[1].Value)
		if err != nil {
			return nil, err
		}

		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &CIPFetcher{},
			matcher: matcher,
		}, nil
	case "req_cip_hash_in":
		matcher, err := NewHashMatcher(node.Args[0].Value, false)
		if err != nil {
			return nil, err
		}

		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &CIPFetcher{},
			matcher: matcher,
		}, nil
	case "req_proto_match":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			fetcher: &ProtoFetcher{},
			matcher: NewExactMatcher(node.Args[0].Value, true),
		}, nil
	case "req_proto_secure":
		return &SecureProtoMatcher{}, nil
	case "req_host_in":
		matcher, err := NewHostMatcher(node.Args[0].Value)
		if err != nil {
			return nil, err
		}

		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &HostFetcher{},
			matcher: matcher,
		}, nil
	case "req_host_tag_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &HostTagFetcher{},
			matcher: NewInMatcher(node.Args[0].Value, true),
		}, nil
	case "req_host_regmatch":
		reg, err := regexp.Compile(node.Args[0].Value)
		if err != nil {
			return nil, fmt.Errorf("compile regexp err %s", err)
		}
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &HostFetcher{},
			matcher: NewRegMatcher(reg),
		}, nil
	case "req_host_suffix_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &HostFetcher{},
			matcher: NewSuffixInMatcher(node.Args[0].Value, true),
		}, nil
	case "req_path_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &PathFetcher{},
			matcher: NewInMatcher(node.Args[0].Value, node.Args[1].ToBool()),
		}, nil
	case "req_path_prefix_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &PathFetcher{},
			matcher: NewPrefixInMatcher(node.Args[0].Value, node.Args[1].ToBool()),
		}, nil
	case "req_path_suffix_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &PathFetcher{},
			matcher: NewSuffixInMatcher(node.Args[0].Value, node.Args[1].ToBool()),
		}, nil
	case "req_path_element_prefix_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &PathFetcher{},
			matcher: NewPathElementPrefixMatcher(node.Args[0].Value, node.Args[1].ToBool()),
		}, nil
	case "req_path_regmatch":
		reg, err := regexp.Compile(node.Args[0].Value)
		if err != nil {
			return nil, fmt.Errorf("compile regexp err %s", err)
		}
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &PathFetcher{},
			matcher: NewRegMatcher(reg),
		}, nil
	case "req_path_contain":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &PathFetcher{},
			matcher: NewContainMatcher(node.Args[0].Value, node.Args[1].ToBool()),
		}, nil
	case "req_url_regmatch":
		reg, err := regexp.Compile(node.Args[0].Value)
		if err != nil {
			return nil, fmt.Errorf("compile regexp err %s", err)
		}
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &UrlFetcher{},
			matcher: NewRegMatcher(reg),
		}, nil
	case "req_query_key_in":
		return &PrimitiveCond{
			name: node.Fun.Name,
			node: node,
			fetcher: &QueryKeyInFetcher{
				keys: strings.Split(node.Args[0].Value, "|"),
			},
			matcher: &BypassMatcher{},
		}, nil
	case "req_query_exist":
		return &QueryExistMatcher{}, nil
	case "req_query_key_prefix_in":
		return &PrimitiveCond{
			name: node.Fun.Name,
			node: node,
			fetcher: &QueryKeyPrefixInFetcher{
				keys: strings.Split(node.Args[0].Value, "|"),
			},
			matcher: &BypassMatcher{},
		}, nil
	case "req_query_value_in":
		return &PrimitiveCond{
			name: node.Fun.Name,
			node: node,
			fetcher: &QueryValueFetcher{
				key: node.Args[0].Value,
			},
			matcher: NewInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_query_value_prefix_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &QueryValueFetcher{node.Args[0].Value},
			matcher: NewPrefixInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_query_value_suffix_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &QueryValueFetcher{node.Args[0].Value},
			matcher: NewSuffixInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_query_value_regmatch":
		reg, err := regexp.Compile(node.Args[1].Value)
		if err != nil {
			return nil, fmt.Errorf("compile regexp err %s", err)
		}

		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &QueryValueFetcher{node.Args[0].Value},
			matcher: NewRegMatcher(reg),
		}, nil
	case "req_query_value_contain":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &QueryValueFetcher{node.Args[0].Value},
			matcher: NewContainMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_query_value_hash_in":
		matcher, err := NewHashMatcher(node.Args[1].Value, node.Args[2].ToBool())
		if err != nil {
			return nil, err
		}

		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &QueryValueFetcher{node.Args[0].Value},
			matcher: matcher,
		}, nil
	case "req_cookie_key_in":
		return &PrimitiveCond{
			name: node.Fun.Name,
			node: node,
			fetcher: &CookieKeyInFetcher{
				keys: strings.Split(node.Args[0].Value, "|"),
			},
			matcher: &BypassMatcher{},
		}, nil
	case "req_cookie_value_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &CookieValueFetcher{node.Args[0].Value},
			matcher: NewInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_cookie_value_prefix_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &CookieValueFetcher{node.Args[0].Value},
			matcher: NewPrefixInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_cookie_value_suffix_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &CookieValueFetcher{node.Args[0].Value},
			matcher: NewSuffixInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_cookie_value_contain":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &CookieValueFetcher{node.Args[0].Value},
			matcher: NewContainMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_cookie_value_hash_in":
		matcher, err := NewHashMatcher(node.Args[1].Value, node.Args[2].ToBool())
		if err != nil {
			return nil, err
		}

		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &CookieValueFetcher{node.Args[0].Value},
			matcher: matcher,
		}, nil
	case "req_port_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &PortFetcher{},
			matcher: NewInMatcher(node.Args[0].Value, false),
		}, nil
	case "req_tag_match":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &TagFetcher{key: node.Args[0].Value},
			matcher: &HasTagMatcher{value: node.Args[1].Value},
		}, nil
	case "req_ua_regmatch":
		reg, err := regexp.Compile(node.Args[0].Value)
		if err != nil {
			return nil, fmt.Errorf("compile regexp err %s", err)
		}

		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &UAFetcher{},
			matcher: NewRegMatcher(reg),
		}, nil
	case "req_header_key_in":
		return &PrimitiveCond{
			name: node.Fun.Name,
			node: node,
			fetcher: &HeaderKeyInFetcher{
				keys: strings.Split(node.Args[0].Value, "|"),
			},
			matcher: &BypassMatcher{},
		}, nil
	case "req_header_value_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &HeaderValueFetcher{node.Args[0].Value},
			matcher: NewInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_header_value_prefix_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &HeaderValueFetcher{node.Args[0].Value},
			matcher: NewPrefixInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_header_value_suffix_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &HeaderValueFetcher{node.Args[0].Value},
			matcher: NewSuffixInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_header_value_regmatch":
		reg, err := regexp.Compile(node.Args[1].Value)
		if err != nil {
			return nil, fmt.Errorf("compile regexp err %s", err)
		}

		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &HeaderValueFetcher{node.Args[0].Value},
			matcher: NewRegMatcher(reg),
		}, nil
	case "req_header_value_contain":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &HeaderValueFetcher{node.Args[0].Value},
			matcher: NewContainMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_header_value_hash_in":
		matcher, err := NewHashMatcher(node.Args[1].Value, node.Args[2].ToBool())
		if err != nil {
			return nil, err
		}

		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &HeaderValueFetcher{node.Args[0].Value},
			matcher: matcher,
		}, nil
	case "req_method_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &MethodFetcher{},
			matcher: NewInMatcher(node.Args[0].Value, true),
		}, nil
	case "res_code_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &ResCodeFetcher{},
			matcher: NewInMatcher(node.Args[0].Value, false),
		}, nil
	case "res_header_key_in":
		return &PrimitiveCond{
			name: node.Fun.Name,
			node: node,
			fetcher: &ResHeaderKeyInFetcher{
				keys: strings.Split(node.Args[0].Value, "|"),
			},
			matcher: &BypassMatcher{},
		}, nil
	case "res_header_value_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &ResHeaderValueFetcher{node.Args[0].Value},
			matcher: NewInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil

	case "ses_vip_range":
		matcher, err := NewIPMatcher(node.Args[0].Value, node.Args[1].Value)
		if err != nil {
			return nil, err
		}

		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &VIPFetcher{},
			matcher: matcher,
		}, nil
	case "ses_sip_range":
		matcher, err := NewIPMatcher(node.Args[0].Value, node.Args[1].Value)
		if err != nil {
			return nil, err
		}

		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &SIPFetcher{},
			matcher: matcher,
		}, nil
	case "ses_tls_sni_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &SniFetcher{},
			matcher: NewInMatcher(node.Args[0].Value, true),
		}, nil
	case "ses_tls_client_auth":
		return &ClientAuthMatcher{}, nil
	case "ses_tls_client_ca_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &ClientCANameFetcher{},
			matcher: NewInMatcher(node.Args[0].Value, false),
		}, nil
	case "req_context_value_in":
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &ContextValueFetcher{node.Args[0].Value},
			matcher: NewInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil

	case "bfe_time_range":
		matcher, err := NewTimeMatcher(node.Args[0].Value, node.Args[1].Value)
		if err != nil {
			return nil, err
		}
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &BfeTimeFetcher{},
			matcher: matcher,
		}, nil

	case "bfe_periodic_time_range":
		matcher, err := NewPeriodicTimeMatcher(node.Args[0].Value, node.Args[1].Value, node.Args[2].Value)
		if err != nil {
			return nil, err
		}
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &BfeTimeFetcher{},
			matcher: matcher,
		}, nil

	case "time_weekday_in":
		// optional second arg (STRING): military time zone letter, "Z" (UTC) if omitted
		zone := "Z"
		if len(node.Args) == 2 {
			zone = node.Args[1].Value
		}
		matcher, err := NewWeekdayMatcher(node.Args[0].Value, zone)
		if err != nil {
			return nil, fmt.Errorf("time_weekday_in \"%s\": %s", node.Args[0].Value, err.Error())
		}
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &BfeTimeFetcher{},
			matcher: matcher,
		}, nil

	case "req_body_json_in":
		return &PrimitiveCond{
			name: node.Fun.Name,
			node: node,
			fetcher: &ReqBodyJsonFetcher{
				path: node.Args[0].Value,
			},
			matcher: NewInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_body_json_prefix_in":
		return &PrimitiveCond{
			name: node.Fun.Name,
			node: node,
			fetcher: &ReqBodyJsonFetcher{
				path: node.Args[0].Value,
			},
			matcher: NewPrefixInMatcher(node.Args[1].Value, node.Args[2].ToBool()),
		}, nil
	case "req_body_larger_than":
		size, err := strconv.ParseInt(node.Args[0].Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("req_body_larger_than: invalid size %s", node.Args[0].Value)
		}
		if size < 0 {
			return nil, fmt.Errorf("req_body_larger_than: size should not be negative")
		}
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &ContentLengthFetcher{},
			matcher: &GtInt64Matcher{threshold: size},
		}, nil
	case "req_body_less_than":
		size, err := strconv.ParseInt(node.Args[0].Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("req_body_less_than: invalid size %s", node.Args[0].Value)
		}
		if size < 0 {
			return nil, fmt.Errorf("req_body_less_than: size should not be negative")
		}
		return &PrimitiveCond{
			name:    node.Fun.Name,
			node:    node,
			fetcher: &ContentLengthFetcher{},
			matcher: &LtInt64Matcher{threshold: size},
		}, nil

	case "req_ai_intent_in":
		// optional third arg (FLOAT): rule-level confidence threshold.
		// -1 is the sentinel for "omitted": the current per-question
		// MinConfidence of the question applies.
		minConf := -1.0
		if len(node.Args) == 3 {
			v, err := strconv.ParseFloat(node.Args[2].Value, 64)
			if err != nil {
				return nil, fmt.Errorf("req_ai_intent_in: invalid min confidence %s", node.Args[2].Value)
			}
			minConf = v
		}
		return &AiIntentCond{
			question: node.Args[0].Value,
			options:  node.Args[1].Value,
			minConf:  minConf,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported primitive %s", node.Fun.Name)
	}
}
