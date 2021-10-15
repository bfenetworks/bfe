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
	"github.com/bfenetworks/bfe/bfe_util/access_log"
	"reflect"
	"testing"
)

func TestConfModWafCheck(t *testing.T) {
	type fields struct {
		Basic struct {
			ProductRulePath string
		}
		Log access_log.LogConfig
	}
	type args struct {
		confRoot string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "normal",
			fields: fields{
				Basic: struct {
					ProductRulePath string
				}{
					ProductRulePath: "mod_waf/waf_rule.data",
				},
				Log: access_log.LogConfig{
					LogPrefix:   "waf",
					LogDir:      "../log",
					RotateWhen:  "NEXTHOUR",
					BackupCount: 24,
				},
			},
			args: args{
				confRoot: "mod_waf.conf",
			},
			wantErr: false,
		},
		{
			name: "normal-empty rule path ",
			fields: fields{
				Basic: struct {
					ProductRulePath string
				}{
					ProductRulePath: "",
				},
				Log: access_log.LogConfig {
					LogPrefix:   "waf",
					LogDir:      "../log",
					RotateWhen:  "NEXTHOUR",
					BackupCount: 24,
				},
			},
			args: args{
				confRoot: "mod_waf.conf",
			},
			wantErr: false,
		},
		{
			name: "normal-empty concurrency",
			fields: fields{
				Basic: struct {
					ProductRulePath string
				}{
					ProductRulePath: "mod_waf/waf_rule.data",
				},
				Log: access_log.LogConfig {
					LogPrefix:   "waf",
					LogDir:      "../log",
					RotateWhen:  "NEXTHOUR",
					BackupCount: 24,
				},
			},
			args: args{
				confRoot: "mod_waf.conf",
			},
			wantErr: false,
		},
		{
			name: "abnormal-empty log prefix",
			fields: fields{
				Basic: struct {
					ProductRulePath string
				}{
					ProductRulePath: "mod_waf/waf_rule.data",
				},
				Log: access_log.LogConfig {
					LogPrefix:   "",
					LogDir:      "../log",
					RotateWhen:  "NEXTHOUR",
					BackupCount: 24,
				},
			},
			args: args{
				confRoot: "mod_waf.conf",
			},
			wantErr: true,
		},
		{
			name: "abnormal-empty LogDir",
			fields: fields{
				Basic: struct {
					ProductRulePath string
				}{
					ProductRulePath: "mod_waf/waf_rule.data",
				},
				Log: access_log.LogConfig {
					LogPrefix:   "waf",
					LogDir:      "",
					RotateWhen:  "NEXTHOUR",
					BackupCount: 24,
				},
			},
			args: args{
				confRoot: "mod_waf.conf",
			},
			wantErr: true,
		},
		{
			name: "abnormal-empty Backup",
			fields: fields{
				Basic: struct {
					ProductRulePath string
				}{
					ProductRulePath: "mod_waf/waf_rule.data",
				},
				Log: access_log.LogConfig {
					LogPrefix:   "waf",
					LogDir:      "",
					RotateWhen:  "NEXTHOUR",
					BackupCount: 0,
				},
			},
			args: args{
				confRoot: "mod_waf.conf",
			},
			wantErr: true,
		},
		{
			name: "abnormal-empty when",
			fields: fields{
				Basic: struct {
					ProductRulePath string
				}{
					ProductRulePath: "mod_waf/waf_rule.data",
				},
				Log: access_log.LogConfig {
					LogPrefix:   "waf",
					LogDir:      "../log",
					RotateWhen:  "HHHH",
					BackupCount: 24,
				},
			},
			args: args{
				confRoot: "mod_waf.conf",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ConfModWaf{
				Basic: tt.fields.Basic,
				Log:   tt.fields.Log,
			}
			if err := cfg.Check(tt.args.confRoot); (err != nil) != tt.wantErr {
				t.Errorf("ConfModWaf.Check() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfLoad(t *testing.T) {
	type args struct {
		filePath string
		confRoot string
	}
	tests := []struct {
		name    string
		args    args
		want    *ConfModWaf
		wantErr bool
	}{
		{
			name: "normal",
			args: args{
				filePath: "./testdata/mod_waf.conf",
				confRoot: "./testdata",
			},
			want: &ConfModWaf{
				Basic: struct {
					ProductRulePath string
				}{
					ProductRulePath: "testdata/mod_waf/waf_rule.data",
				},
				Log: access_log.LogConfig {
					LogPrefix:   "waf",
					LogDir:      "log",
					RotateWhen:  "NEXTHOUR",
					BackupCount: 24,
				},
			},
			wantErr: false,
		},
		{
			name: "normal",
			args: args{
				filePath: "./testdata/notexist.conf",
				confRoot: "./testdata",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConfLoad(tt.args.filePath, tt.args.confRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConfLoad() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConfLoad() = %v, want %v", got, tt.want)
			}
		})
	}
}
