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
)

func TestConfModMD_Check(t *testing.T) {
	type fields struct {
		Basic struct{ ProductRulePath string }
		Log   struct{ OpenDebug bool }
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
			name: "",
			fields: fields{
				Basic: struct{ ProductRulePath string }{
					ProductRulePath: "",
				},
				Log: struct{ OpenDebug bool }{
					OpenDebug: false,
				},
			},
			args: args{
				confRoot: "",
			},
			wantErr: false,
		},
		{
			name: "",
			fields: fields{
				Basic: struct{ ProductRulePath string }{
					ProductRulePath: "./testdata/mod_markdown.conf",
				},
				Log: struct{ OpenDebug bool }{
					OpenDebug: false,
				},
			},
			args: args{
				confRoot: "",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ConfModMarkdown{
				Basic: tt.fields.Basic,
				Log:   tt.fields.Log,
			}
			if err := cfg.Check(tt.args.confRoot); (err != nil) != tt.wantErr {
				t.Errorf("ConfModMarkdown.Check() error = %v, wantErr %v", err, tt.wantErr)
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
		want    *ConfModMarkdown
		wantErr bool
	}{
		{
			name: "",
			args: args{
				filePath: "./testdata/mod_markdown.conf",
				confRoot: "./testdata",
			},
			want: &ConfModMarkdown{
				Basic: struct{ ProductRulePath string }{
					ProductRulePath: "testdata/mod_markdown.data",
				},
				Log: struct{ OpenDebug bool }{
					OpenDebug: true,
				},
			},
			wantErr: false,
		},
		{
			name: "",
			args: args{
				filePath: "./testdata/not_exists.conf",
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
