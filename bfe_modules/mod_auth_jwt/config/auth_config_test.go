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

package config

import (
	"testing"
)

import (
	"github.com/baidu/bfe/bfe_util"
)

func TestAuthConfig_BuildSecret(t *testing.T) {
	config := &AuthConfig{SecretPath: bfe_util.ConfPathProc("secret_jws_valid_1.key", confRoot)}
	err := config.BuildSecret()
	if err != nil {
		t.Error(err)
	}
}
