// Copyright (c) 2020 The BFE Authors.
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
package mod_waf

import (
	"reflect"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

func TestProductWafRuleConfLoad(t *testing.T) {
	cond, err := condition.Build("default_t()")
	if err != nil {
		t.FailNow()
	}
	type args struct {
		fileName string
	}
	tests := []struct {
		name    string
		args    args
		want    productWafRuleConfig
		wantErr bool
	}{
		{
			name: "normal",
			args: args{
				fileName: "./testdata/waf_rule.data",
			},
			want: productWafRuleConfig{
				Version: "2019-12-10184356",
				Config: map[string]*ruleList{
					"example_product": &ruleList{&wafRule{
						Cond:       cond,
						CheckRules: []string{},
						BlockRules: []string{
							"RuleBashCmd",
						},
					}},
				},
			},
			wantErr: false,
		},
		{
			name: "not_exist_waf_rule",
			args: args{
				fileName: "./testdata/not_exist_waf_rule.data",
			},
			want:    *new(productWafRuleConfig),
			wantErr: true,
		},
		{
			name: "waf_rule_invalid_json",
			args: args{
				fileName: "./testdata/waf_rule_invalid_json.data",
			},
			want:    *new(productWafRuleConfig),
			wantErr: true,
		},
		{
			name: "waf_rule_no_version",
			args: args{
				fileName: "./testdata/waf_rule_no_version.data",
			},
			want:    *new(productWafRuleConfig),
			wantErr: true,
		},
		{
			name: "waf_rule_no_config",
			args: args{
				fileName: "./testdata/waf_rule_no_config.data",
			},
			want:    *new(productWafRuleConfig),
			wantErr: true,
		},
		{
			name: "waf_rule_invalid_json",
			args: args{
				fileName: "./testdata/waf_rule_invalid_json.data",
			},
			want:    *new(productWafRuleConfig),
			wantErr: true,
		},
		{
			name: "waf_rule_invalid_cond",
			args: args{
				fileName: "./testdata/waf_rule_invalid_cond.data",
			},
			want:    *new(productWafRuleConfig),
			wantErr: true,
		},
		{
			name: "waf_rule_both_empty",
			args: args{
				fileName: "./testdata/waf_rule_both_empty.data",
			},
			want:    *new(productWafRuleConfig),
			wantErr: true,
		},
		{
			name: "waf_rule_both_block_rules",
			args: args{
				fileName: "./testdata/waf_rule_both_block_rules.data",
			},
			want:    *new(productWafRuleConfig),
			wantErr: true,
		},
		{
			name: "waf_rule_both_check_rules",
			args: args{
				fileName: "./testdata/waf_rule_both_check_rules.data",
			},
			want:    *new(productWafRuleConfig),
			wantErr: true,
		},
		{
			name: "waf_rule_both_nil",
			args: args{
				fileName: "./testdata/waf_rule_both_nil.data",
			},
			want:    *new(productWafRuleConfig),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProductWafRuleConfLoad(tt.args.fileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProductWafRuleConfLoad() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ProductWafRuleConfLoad() = %v, want %v", got, tt.want)
			}
		})
	}
}
