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
	"testing"

	"github.com/bfenetworks/bfe/bfe_basic/condition"
)

func Test_ruleConvert(t *testing.T) {
	var ruleMap = make(map[string]*condition.Condition)
	hostRule := "req_host_in(\"www.baidu.con\")"
	vipRule := "req_vip_in(\"1.1.1.1\")"
	var ruleStrs = []string{hostRule, vipRule}
	for _, str := range ruleStrs {
		con, err := condition.Build(str)
		if err != nil {
			t.FailNow()
		}
		ruleMap[str] = &con
	}

	type args struct {
		ruleFile *markdownRuleFile
	}
	tests := []struct {
		name    string
		args    args
		want    *markdownRule
		wantErr bool
	}{
		{
			name: "host rule",
			args: args{
				ruleFile: &markdownRuleFile{
					Cond: hostRule,
				},
			},
			want: &markdownRule{
				Cond: ruleMap[hostRule],
			},
			wantErr: false,
		},
		{
			name: "vip rule",
			args: args{
				ruleFile: &markdownRuleFile{
					Cond: vipRule,
				},
			},
			want: &markdownRule{
				Cond: ruleMap[vipRule],
			},
			wantErr: false,
		},
		{
			name: "nil rule",
			args: args{
				ruleFile: nil,
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ruleConvert(tt.args.ruleFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("ruleConvert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ruleConvert() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_rulesConvert(t *testing.T) {
	ruleStr := "req_host_in(\"www.baidu.com\")"
	ruleCond, _ := condition.Build(ruleStr)
	type args struct {
		ruleFiles *markdownRuleFiles
	}
	tests := []struct {
		name    string
		args    args
		want    *markdownRules
		wantErr bool
	}{
		{
			name: "",
			args: args{
				ruleFiles: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "",
			args: args{
				ruleFiles: &markdownRuleFiles{},
			},
			want:    &markdownRules{},
			wantErr: false,
		},
		{
			name: "",
			args: args{
				ruleFiles: &markdownRuleFiles{&markdownRuleFile{
					Cond: ruleStr,
				}},
			},
			want: &markdownRules{&markdownRule{
				Cond: &ruleCond,
			}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := rulesConvert(tt.args.ruleFiles)
			if (err != nil) != tt.wantErr {
				t.Errorf("rulesConvert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("rulesConvert() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_mdRuleCheck(t *testing.T) {
	type args struct {
		rule *markdownRuleFile
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "",
			args: args{
				rule: &markdownRuleFile{
					Cond: "",
				},
			},
			wantErr: true,
		},
		{
			name: "",
			args: args{
				rule: nil,
			},
			wantErr: true,
		},
		{
			name: "normal",
			args: args{
				rule: &markdownRuleFile{
					Cond: "req_host_in(\"1.1.1.1\")",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := mdRuleCheck(tt.args.rule); (err != nil) != tt.wantErr {
				t.Errorf("mdRuleCheck() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_rulesCheck(t *testing.T) {
	type args struct {
		mdRuleFiles *markdownRuleFiles
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "nil",
			args: args{
				mdRuleFiles: nil,
			},
			wantErr: true,
		},
		{
			name: "empty md rules",
			args: args{
				mdRuleFiles: &markdownRuleFiles{},
			},
			wantErr: false,
		},
		{
			name: "invalid rules",
			args: args{
				mdRuleFiles: &markdownRuleFiles{&markdownRuleFile{
					Cond: "",
				}},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := rulesCheck(tt.args.mdRuleFiles); (err != nil) != tt.wantErr {
				t.Errorf("rulesCheck() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_productRulesFileCheck(t *testing.T) {
	type args struct {
		cfg *productRulesFile
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "nil",
			args: args{
				cfg: nil,
			},
			wantErr: true,
		},
		{
			name: "empty cfg",
			args: args{
				cfg: &productRulesFile{},
			},
			wantErr: false,
		},
		{
			name: "nil rule",
			args: args{
				cfg: &productRulesFile{
					"hello": nil,
				},
			},
			wantErr: true,
		},
		{
			name: "nil rule",
			args: args{
				cfg: &productRulesFile{
					"hello": &markdownRuleFiles{&markdownRuleFile{
						Cond: "",
					}},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := productRulesFileCheck(tt.args.cfg); (err != nil) != tt.wantErr {
				t.Errorf("productRulesFileCheck() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_productRuleFileCheck(t *testing.T) {
	type args struct {
		cfg productRuleConfFile
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "empty version",
			args: args{
				cfg: productRuleConfFile{
					Version: "",
					Config:  &productRulesFile{},
				},
			},
			wantErr: true,
		},
		{
			name: "nil config",
			args: args{
				cfg: productRuleConfFile{
					Version: "123",
					Config:  nil,
				},
			},
			wantErr: true,
		},
		{
			name: "nil config",
			args: args{
				cfg: productRuleConfFile{
					Version: "123",
					Config: &productRulesFile{
						"hello": &markdownRuleFiles{&markdownRuleFile{
							Cond: "",
						}},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "normal config",
			args: args{
				cfg: productRuleConfFile{
					Version: "123",
					Config: &productRulesFile{
						"hello": &markdownRuleFiles{&markdownRuleFile{
							Cond: "req_host_in(\"www.baidu.com\")",
						}},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := productRuleFileCheck(tt.args.cfg); (err != nil) != tt.wantErr {
				t.Errorf("productRuleFileCheck() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProductRuleConfLoad(t *testing.T) {
	rule, _ := condition.Build("req_path_in(\"/md\", false)")
	defaultRule, _ := condition.Build("req_path_in(\"/default\", false)")
	githubRule, _ := condition.Build("req_path_in(\"/github\", false)")
	type args struct {
		fileName string
	}
	tests := []struct {
		name    string
		args    args
		want    productRuleConf
		wantErr bool
	}{
		{
			name: "normal",
			args: args{
				fileName: "./testdata/mod_markdown.data",
			},
			want: productRuleConf{
				Version: "123",
				Config: productRules{
					"unittest2": &markdownRules{&markdownRule{
						Cond: &rule,
					},
					},
					"unittest": &markdownRules{&markdownRule{
						Cond: &defaultRule,
					},
						&markdownRule{
							Cond: &githubRule,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "noversion",
			args: args{
				fileName: "./testdata/mod_md_noversion.data",
			},
			want:    productRuleConf{},
			wantErr: true,
		},
		{
			name: "invalid cond",
			args: args{
				fileName: "./testdata/mod_md_invalid_cond.data",
			},
			want:    productRuleConf{},
			wantErr: true,
		},
		{
			name: "invalid action",
			args: args{
				fileName: "./testdata/mod_md_invalid_action.data",
			},
			want:    productRuleConf{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ProductRuleConfLoad(tt.args.fileName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProductRuleConfLoad() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ProductRuleConfLoad() = %v, want %v", got, tt.want)
			}
		})
	}
}
