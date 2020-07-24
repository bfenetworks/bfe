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

package mod_userid

import (
	"reflect"
	"testing"
)

import (
	"github.com/bfenetworks/bfe/bfe_util"
)

func TestConfLoad(t *testing.T) {
	defaultDataPath := "mod_userid/userid_rule.data"

	type args struct {
		filePath string
		confRoot string
	}
	tests := []struct {
		name    string
		args    args
		want    *ConfModUserID
		wantErr bool
	}{
		{
			name: "case:succ",
			args: args{
				filePath: "./testdata/mod_userid/mod_userid.conf",
				confRoot: "./testdata",
			},
			want: &ConfModUserID{
				Basic: struct {
					DataPath string
				}{
					DataPath: bfe_util.ConfPathProc(defaultDataPath, "./testdata"),
				},
				Log: struct {
					OpenDebug bool
				}{
					OpenDebug: true,
				},
			},
			wantErr: false,
		},
		{
			name: "case: file not existed",
			args: args{
				filePath: "./testdata/mod_userid/mod_userid.conf_not_existed",
				confRoot: "./testdata",
			},
			wantErr: true,
		},
		{
			name: "case: succ with default path",
			args: args{
				filePath: "./testdata/mod_userid/mod_userid_default.conf",
				confRoot: "./testdata",
			},
			want: &ConfModUserID{
				Basic: struct {
					DataPath string
				}{
					DataPath: bfe_util.ConfPathProc(defaultDataPath, "./testdata"),
				},
				Log: struct {
					OpenDebug bool
				}{
					OpenDebug: true,
				},
			},
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
