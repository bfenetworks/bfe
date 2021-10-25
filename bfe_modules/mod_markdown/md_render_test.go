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
	"io/ioutil"
	"reflect"
	"testing"
)

func TestMDRender_Render(t *testing.T) {
	case0md, _ := ioutil.ReadFile("./testdata/testcase0.md")
	defaultcase0, _ := ioutil.ReadFile("./testdata/testcase0_default.output")
	type args struct {
		src []byte
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "empty src",
			args: args{
				src: []byte{},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "normal",
			args: args{
				src: []byte("# hello"),
			},
			want:    []byte("<h1>hello</h1>\n"),
			wantErr: false,
		},
		{
			name: "normal",
			args: args{
				src: case0md,
			},
			want:    defaultcase0,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.args.src)
			if (err != nil) != tt.wantErr {
				t.Errorf("MDRender.Render() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MDRender.Render() = %v, want %v", string(got), string(tt.want))
			}
		})
	}
}
