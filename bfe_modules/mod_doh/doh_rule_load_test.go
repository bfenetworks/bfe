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

package mod_doh

import (
	"testing"
)

func TestDohConfLoadCase(t *testing.T) {
	staticConf, err := DohConfLoad("./testdata/mod_doh/doh_rule.data")
	if err != nil {
		t.Errorf("DohConfLoad() error: %v", err)
		return
	}

	if len(*staticConf.Config["unittest"]) != 1 {
		t.Errorf("Length of static rule should be 1 not %d", len(*staticConf.Config["unittest"]))
	}
}