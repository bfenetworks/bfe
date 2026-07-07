// Copyright (c) 2026 The BFE Authors.
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

package mod_session_sticky

import (
	"testing"
)

func TestProductRuleConfLoad(t *testing.T) {
	type args struct {
		filename string
	}
	tests := []struct {
		name    string
		args    args
		want    ProductRuleConf
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "normal",
			args: args{
				filename: "./test_data/mod_session_sticky.data",
			},
			want:    ProductRuleConf{},
			wantErr: false,
		},
		{
			name: "err length",
			args: args{
				filename: "./test_data/mod_session_sticky_err.data",
			},
			want:    ProductRuleConf{},
			wantErr: true,
		},
		{
			name: "err type",
			args: args{
				filename: "./test_data/mod_session_sticky_err_1.data",
			},
			want:    ProductRuleConf{},
			wantErr: true,
		},
		{
			name: "err type",
			args: args{
				filename: "./test_data/mod_session_sticky_err_2.data",
			},
			want:    ProductRuleConf{},
			wantErr: true,
		},
		{
			name: "default val",
			args: args{
				filename: "./test_data/mod_session_sticky_default.data",
			},
			want:    ProductRuleConf{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ProductRuleConfLoad(tt.args.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProductRuleConfLoad() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
