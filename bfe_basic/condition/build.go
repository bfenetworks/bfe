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
	"strings"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition/parser"
)

func Build(condStr string) (Condition, error) {
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
			matcher: NewInMatcher(node.Args[1].Value, false),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported primitive %s", node.Fun.Name)
	}
}
