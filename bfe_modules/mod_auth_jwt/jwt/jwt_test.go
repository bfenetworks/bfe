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

package jwt

import (
	"fmt"
	"io/ioutil"
	"testing"
)

import (
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/config"
)

var (
	conf = new(config.AuthConfig)

	jwsAlgSet = []string{
		"HS256", "HS384", "HS512", "RS256",
		"RS384", "RS512", "ES256", "ES384",
		"ES512", "PS256", "PS384", "PS512",
	}

	jweAlgSet = []string{
		"dir", "RSA1_5", "RSA-OAEP", "RSA-OAEP-256",
		"A128KW", "A192KW", "A256KW", "A128GCMKW",
		"A192GCMKW", "A256GCMKW", "ECDH-ES",
		"ECDH-ES+A128KW", "ECDH-ES+A192KW",
		"ECDH-ES+A256KW", "PBES2-HS256+A128KW",
		"PBES2-HS384+A192KW", "PBES2-HS512+A256KW",
	}
)

func TestJWSValidate(t *testing.T) {
	for _, name := range jwsAlgSet {
		tokenPath := fmt.Sprintf("./../testdata/mod_auth_jwt/test_jws_%s.txt", name)
		conf.SecretPath = fmt.Sprintf("./../testdata/mod_auth_jwt/secret_test_jws_%s.key", name)

		token, _ := ioutil.ReadFile(tokenPath)
		err := conf.BuildSecret()
		if err != nil {
			t.Error(err)
		}

		mJWT, err := NewJWT(string(token), conf)
		if err != nil {
			t.Error(name, err)
		}

		if err := mJWT.Validate(); err != nil {
			t.Error(name, err)
		}
	}
}

func TestJWEValidate(t *testing.T) {
	for _, name := range jweAlgSet {
		tokenPath := fmt.Sprintf("./../testdata/mod_auth_jwt/test_jwe_%s_A128GCM.txt", name)
		conf.SecretPath = fmt.Sprintf("./../testdata/mod_auth_jwt/secret_test_jwe_%s_A128GCM.key", name)

		token, _ := ioutil.ReadFile(tokenPath)
		err := conf.BuildSecret()
		if err != nil {
			t.Error(err)
		}

		mJWT, err := NewJWT(string(token), conf)
		if err != nil {
			t.Error(name, err)
		}

		if err := mJWT.Validate(); err != nil {
			t.Error(name, err)
		}
	}
}
