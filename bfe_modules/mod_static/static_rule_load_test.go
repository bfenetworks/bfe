// Copyright (c) 2019 Baidu, Inc.
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

package mod_static

import (
	"testing"
)

func TestStaticConfLoad_1(t *testing.T) {
	staticConf, err := StaticConfLoad("./testdata/mod_static/static_rule.data")
	if err != nil {
		t.Errorf("StaticConfLoad() error: %v", err)
		return
	}

	if len(*staticConf.Config["unittest"]) != 1 {
		t.Errorf("Length of static rule should be 1 not %d", len(*staticConf.Config["unittest"]))
	}
}

func TestStaticConfLoad_2(t *testing.T) {
	_, err := StaticConfLoad("./testdata/mod_static/static_rule.data.cmd_error")
	if err == nil || err.Error() != "Config: invalid product rules:unittest, "+
		"StaticRule: 0, Action: invalid cmd: INDEX" {
		t.Errorf("StaticConfLoad() error should be \"\", not %v", err)
	}
}
