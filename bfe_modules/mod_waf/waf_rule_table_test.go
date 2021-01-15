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
	"sync"
	"testing"
)

func TestNewWarRuleTable(t *testing.T) {
	tests := []struct {
		name string
		want *WarRuleTable
	}{
		{
			name: "normal",
			want: &WarRuleTable{
				lock:        sync.RWMutex{},
				version:     "",
				productRule: make(productWafRule),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewWarRuleTable(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewWarRuleTable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWarRuleTableUpdate(t *testing.T) {
	config := map[string]*ruleList{
		"example": &ruleList{&wafRule{
			Cond:       nil,
			BlockRules: []string{"RuleBashCmd"},
			CheckRules: []string{},
		}},
	}
	type fields struct {
		version     string
		productRule productWafRule
	}
	type args struct {
		ruleConf *productWafRuleConfig
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "normal",
			fields: fields{
				version:     "ut",
				productRule: make(productWafRule),
			},
			args: args{
				ruleConf: &productWafRuleConfig{
					Version: "utnew",
					Config:  config,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &WarRuleTable{
				version:     tt.fields.version,
				productRule: tt.fields.productRule,
			}
			w.Update(tt.args.ruleConf)
			for key, value := range config {
				realValue, ok := w.Search(key)
				if !ok {
					t.Errorf("missing product[%s] rule", key)
				}
				if !reflect.DeepEqual(value, realValue) {
					t.Errorf("product update, want[%v] got[%v]", value, realValue)
				}
			}
		})
	}
}
