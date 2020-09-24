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

package mod_auth_basic

import (
	"testing"
)

func TestAuthBasicConfLoadCorrect(t *testing.T) {
	auth_basicConf, err := AuthBasicConfLoad("./testdata/mod_auth_basic/auth_basic_rule.data")
	if err != nil {
		t.Errorf("AuthBasicConfLoad() error: %v", err)
		return
	}

	if len(*auth_basicConf.Config["unittest"]) != 1 {
		t.Errorf("Length of auth_basic rule should be 1 not %d", len(*auth_basicConf.Config["unittest"]))
	}
}

func TestAuthBasicConfLoadUserFileEmpty(t *testing.T) {
	_, err := AuthBasicConfLoad("./testdata/mod_auth_basic/auth_basic_rule.data.userfile_empty")
	if err == nil || err.Error() != "Config: invalid product rules:unittest, "+
		"AuthBasicRule: 0, UserFile empty." {
		t.Errorf("AuthBasicConfLoad() error should be \"\", not %v", err)
	}
}
