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
	"io/ioutil"
	"testing"
)

import (
	"gopkg.in/square/go-jose.v2"
)

func TestNewJWE(t *testing.T) {
	token, _ := ioutil.ReadFile("./../testdata/mod_auth_jwt/jwe_valid_1.txt")
	secret, _ := ioutil.ReadFile("./../testdata/mod_auth_jwt/secret_jwe_valid_1.key")
	key := new(jose.JSONWebKey)
	err := key.UnmarshalJSON(secret)
	if err != nil {
		t.Fatal(err)
	}

	mJWE, err := NewJWE(string(token), key)
	if err != nil {
		t.Fatal(err)
	}

	plaintext, _ := mJWE.Plaintext()
	t.Log(string(plaintext))
	t.Log(mJWE.Payload)
}
