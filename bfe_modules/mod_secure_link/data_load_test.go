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
	"testing"
)

func TestDataLoad(t *testing.T) {
	_, err := DataLoad("testdata/mod_secure_link/secure_link_rule1.data")
	if err == nil {
		t.Errorf("want err, got nil")
	}
	t.Log(err)

	data, err := DataLoad("testdata/mod_secure_link/secure_link_rule.data")
	if err != nil {
		t.Errorf("want nil, got %v", err)
		return
	}

	conf := data.Config
	if got := len(conf); got != 2 {
		t.Errorf("want  2, got: %v", got)
		return
	}

	p1 := conf["p1"]
	if got := len(p1); got != 1 {
		t.Errorf("want  1, got: %v", got)
		return
	}
	p11 := p1[0]
	{
		want := "sign"
		if got := p11.ChecksumKey; want != got {
			t.Errorf("want: %v, got: %v", want, got)
			return
		}
	}
	{
		want := "time"
		if got := p11.ExpiresKey; want != got {
			t.Errorf("want: %v, got: %v", want, got)
			return
		}
	}

	p2 := conf["p2"]
	if got := len(p2); got != 1 {
		t.Errorf("want  1, got: %v", got)
		return
	}
	p21 := p2[0]
	{
		want := "md5"
		if got := p21.ChecksumKey; want != got {
			t.Errorf("want: %v, got: %v", want, got)
			return
		}
	}
	{
		want := ""
		if got := p21.ExpiresKey; want != got {
			t.Errorf("want: %v, got: %v", want, got)
			return
		}
	}

}
