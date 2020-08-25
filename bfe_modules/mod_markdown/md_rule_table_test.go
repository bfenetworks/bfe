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
package mod_markdown

import (
	"reflect"
	"sync"
	"testing"
)

func TestNewMdRuleTable(t *testing.T) {
	tests := []struct {
		name string
		want *MarkdownRuleTable
	}{
		{
			name: "",
			want: &MarkdownRuleTable{
				version:      "",
				productRules: map[string]*markdownRules{},
				lock:         sync.RWMutex{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewMarkdownRuleTable(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewMdRuleTable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMdRuleTable_Update(t *testing.T) {
	type fields struct {
		version      string
		productRules productRules
	}
	type args struct {
		conf productRuleConf
	}
	tests := []struct {
		name        string
		fields      fields
		args        args
		wantVersion string
		wantConfig  *productRules
	}{
		{
			name: "",
			fields: fields{
				version:      "",
				productRules: map[string]*markdownRules{},
			},
			args: args{
				conf: productRuleConf{
					Version: "version1",
					Config:  productRules{},
				},
			},
			wantVersion: "version1",
			wantConfig:  &productRules{},
		},
		{
			name: "",
			fields: fields{
				version: "version2",
				productRules: map[string]*markdownRules{
					"hello": nil,
				},
			},
			args: args{
				conf: productRuleConf{
					Version: "version1",
					Config: productRules{
						"newproduct": nil,
					},
				},
			},
			wantVersion: "version1",
			wantConfig: &productRules{
				"newproduct": nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MarkdownRuleTable{
				version:      tt.fields.version,
				productRules: tt.fields.productRules,
				lock:         sync.RWMutex{},
			}
			m.Update(tt.args.conf)
			if m.version != tt.wantVersion {
				t.Errorf("Update() = %v, want %v", m.version, tt.wantVersion)
				return
			}
			if reflect.DeepEqual(m.productRules, tt.wantConfig) {
				t.Errorf("Update() = %v, want %v", m.productRules, tt.wantConfig)
				return
			}
		})
	}
}

func TestMdRuleTable_Search(t *testing.T) {
	type fields struct {
		version      string
		productRules productRules
	}
	type args struct {
		product string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *markdownRules
		want1  bool
	}{
		{
			name: "",
			fields: fields{
				version: "version1",
				productRules: map[string]*markdownRules{
					"product1": {&markdownRule{}},
				},
			},
			args: args{
				product: "product1",
			},
			want:  &markdownRules{&markdownRule{}},
			want1: true,
		},
		{
			name: "",
			fields: fields{
				version: "version1",
				productRules: map[string]*markdownRules{
					"product1": {&markdownRule{}},
				},
			},
			args: args{
				product: "product2",
			},
			want:  nil,
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MarkdownRuleTable{
				version:      tt.fields.version,
				productRules: tt.fields.productRules,
				lock:         sync.RWMutex{},
			}
			got, got1 := m.Search(tt.args.product)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MdRuleTable.Search() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("MdRuleTable.Search() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}
