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

package mod_auth_jwt

import (
	"strings"
	"testing"
)

func TestAuthJWTConfLoad(t *testing.T) {
	authJWTConf, err := AuthJWTConfLoad("./testdata/mod_auth_jwt/auth_jwt_rule.data")
	if err != nil {
		t.Errorf("AuthJWTConfLoad() error: %v", err)
		return
	}

	if len(*authJWTConf.Config["unittest"]) != 1 {
		t.Errorf("Length of auth_jwt rule should be 1 not %d", len(*authJWTConf.Config["unittest"]))
	}
}

func TestAuthJWTConfLoadKeyInvalid(t *testing.T) {
	_, err := AuthJWTConfLoad("./testdata/mod_auth_jwt/auth_jwt_rule.data.key_invalid")
	if err == nil || !strings.Contains(err.Error(), " unknown json web key type 'invalid'") {
		t.Errorf("AuthJWTConfLoad() error: %v", err)
	}
}
