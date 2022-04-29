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

package semver

import "testing"

type testItem struct {
	version string
	valid   bool
}

func TestNewVersion(t *testing.T) {
	tests := []testItem{
		{version: "0.1.0"},
		{version: "1.0.0-0.3.7"},
		{version: "1.0.0-alpha"},
		{version: "1.0.0-alpha+001"},
		{version: "1.0.0-alpha.1"},
		{version: "1.0.0-alpha.beta"},
		{version: "1.0.0-beta"},
		{version: "1.0.0-beta+exp.sha.5114f85"},
		{version: "1.0.0-beta.2"},
		{version: "1.0.0-beta.11"},
		{version: "1.0.0-rc.1"},
		{version: "1.0.0-x.7.z.92"},
		{version: "1.0.0"},
		{version: "1.0.0+20130313144700"},
		{version: "1.8.0-alpha.3"},
		{version: "1.8.0-alpha.3.673+73326ef01d2d7c"},
		{version: "1.9.0"},
		{version: "1.10.0"},
		{version: "1.11.0"},
		{version: "2.0.0"},
		{version: "2.1.0"},
		{version: "2.1.1"},
		{version: "42.0.0"},
	}

	for _, test := range tests {
		v, err := New(test.version)
		if err != nil {
			t.Fatal(err)
		}

		if test.version != v.String() {
			t.Fatal("unexpected")
		}
	}
}

func TestVersionEqual(t *testing.T) {
	v1, err := New("1.2.3")
	if err != nil {
		t.Fatal(err)
	}

	v2, err := New("1.2.3")
	if err != nil {
		t.Fatal(err)
	}

	if !v1.Equal(v2) {
		t.Fatal("unexpected")
	}
}
