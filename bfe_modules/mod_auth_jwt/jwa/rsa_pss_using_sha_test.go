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

package jwa

import (
	"encoding/json"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"io/ioutil"
	"strings"
	"testing"
)

func TestNewPS256(t *testing.T) {
	secret, err := ioutil.ReadFile("./../testdata/mod_auth_jwt/secret_test_jws_PS256.key")
	if err != nil {
		t.Fatal(err)
	}
	token, err := ioutil.ReadFile("./../testdata/mod_auth_jwt/test_jws_PS256.txt")
	if err != nil {
		t.Fatal(err)
	}
	keyMap := make(map[string]interface{})
	if err = json.Unmarshal(secret, &keyMap); err != nil {
		t.Fatal(err)
	}
	mJWK, err := jwk.NewJWK(keyMap)
	if err != nil {
		t.Fatal(err)
	}
	tokens := strings.Split(string(token), ".")
	PS256, err := NewPS256(mJWK)
	if err != nil {
		t.Fatal(err)
	}
	_, err = PS256.Update([]byte(strings.Join(tokens[:2], ".")))
	if err != nil {
		t.Fatal(err)
	}
	sig, _ := jwk.Base64URLDecode(tokens[2])
	if !PS256.Verify(sig) {
		t.Error("wrong signature check result")
	}
}
