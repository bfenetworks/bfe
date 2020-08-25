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

package mod_key_log

import (
	"testing"
)

func TestKeyLogConfLoad_Normal(t *testing.T) {
	config, err := keyLogConfLoad("./testdata/key_log_1.conf")
	if err != nil {
		t.Errorf("get err from keyLogConfLoad():%s", err.Error())
		return
	}

	if len(*config.Config["pn"]) != 1 {
		t.Errorf("len(config.Config['pn']) should be 2")
		return
	}
}

func TestKeyLogConfLoad_ProductIsNull(t *testing.T) {
	_, err := keyLogConfLoad("./testdata/key_log_2.conf")
	if err == nil {
		t.Error("err should not be nil")
		return
	}
}
