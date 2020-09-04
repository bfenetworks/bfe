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
package waf_rule

import (
	"reflect"
	"testing"
)

func TestIsValidRule(t *testing.T) {
	type args struct {
		rule string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "normal",
			args: args{
				rule: RuleBashCmd,
			},
			want: true,
		},
		{
			name: "abnormal",
			args: args{
				rule: "invalid",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidRule(tt.args.rule); got != tt.want {
				t.Errorf("IsValidRule() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewWafRuleTable(t *testing.T) {
	tests := []struct {
		name string
		want *WafRuleTable
	}{
		{
			name: "normal",
			want: &WafRuleTable{
				rules: make(map[string]WafRule),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewWafRuleTable(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewWafRuleTable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWafRuleTable_Init(t *testing.T) {
	type fields struct {
		rules map[string]WafRule
	}
	tests := []struct {
		name   string
		fields fields
	}{
		{
			name: "normal",
			fields: fields{
				rules: map[string]WafRule{
					RuleBashCmd: NewRuleBashCmdExe(),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wr := &WafRuleTable{
				rules: tt.fields.rules,
			}
			wr.Init()
		})
	}
}

func TestWafRuleTable_GetRule(t *testing.T) {
	type fields struct {
		rules map[string]WafRule
	}
	type args struct {
		ruleName string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   WafRule
		want1  bool
	}{
		{
			name: "normal",
			fields: fields{
				rules: map[string]WafRule{
					RuleBashCmd: NewRuleBashCmdExe(),
				},
			},
			args: args{
				ruleName: RuleBashCmd,
			},
			want:  NewRuleBashCmdExe(),
			want1: true,
		},
		{
			name: "abnormal",
			fields: fields{
				rules: map[string]WafRule{
					RuleBashCmd: NewRuleBashCmdExe(),
				},
			},
			args: args{
				ruleName: "SQLInjection",
			},
			want:  nil,
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wr := &WafRuleTable{
				rules: tt.fields.rules,
			}
			got, got1 := wr.GetRule(tt.args.ruleName)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("WafRuleTable.GetRule() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("WafRuleTable.GetRule() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
