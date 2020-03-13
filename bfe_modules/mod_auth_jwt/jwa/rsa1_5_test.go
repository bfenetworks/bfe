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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"io/ioutil"
	"strings"
	"testing"
)

func TestNewRSA15(t *testing.T) {
	token, _ := ioutil.ReadFile(fmt.Sprintf("%s/test_jwe_RSA1_5_A128GCM.txt", relativePath))
	secret, _ := ioutil.ReadFile(fmt.Sprintf("%s/secret_test_jwe_RSA1_5_A128GCM.key", relativePath))
	eCek, _ := base64.RawURLEncoding.DecodeString(strings.Split(string(token), ".")[1])
	keyMap := make(map[string]interface{})
	_ = json.Unmarshal(secret, &keyMap)
	mJWK, _ := jwk.NewJWK(keyMap)
	context, err := NewRSA15(mJWK, nil)
	if err != nil {
		t.Fatal(err)
	}
	cek, err := context.Decrypt(eCek)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(cek)
}
