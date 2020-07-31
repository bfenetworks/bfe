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

package mod_secure_link

import (
	"reflect"
	"testing"
)

func TestConfLoad(t *testing.T) {
	type args struct {
		filePath string
		confRoot string
	}
	tests := []struct {
		name    string
		args    args
		want    *ConfModSecureLink
		wantErr bool
	}{
		{
			name: "case1",
			args: args{
				filePath: "testdata/mod_secure_link/mod_secure_link.conf",
				confRoot: "",
			},
			want: &ConfModSecureLink{
				Basic: struct {
					DataPath string // path of config data (mod_secure_link)
				}{
					DataPath: "mod_secure_link/secure_link.data",
				},
				Log: struct {
					OpenDebug bool
				}{},
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
			if err != nil {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConfLoad() = %v, want %v", got, tt.want)
			}
		})
	}
}
