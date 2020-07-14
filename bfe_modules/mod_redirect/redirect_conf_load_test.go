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

package mod_redirect

import (
	"testing"
)

func TestRedirectConfLoad_1(t *testing.T) {
	config, err := redirectConfLoad("./testdata/redirect_1.conf")
	if err != nil {
		t.Errorf("get err from redirectConfLoad():%s", err.Error())
		return
	}

	if len(*config.Config["pn"]) != 1 {
		t.Errorf("len(config.Config['pn']) should be 2")
		return
	}
}

func TestRedirectConfLoad_2(t *testing.T) {
	_, err := redirectConfLoad("./testdata/redirect_2.conf")
	if err == nil {
		t.Error("err should not be nil")
		return
	}
}
