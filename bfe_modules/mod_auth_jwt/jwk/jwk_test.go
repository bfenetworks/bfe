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

package jwk

import (
	"encoding/json"
	"io/ioutil"
	"testing"
)

func TestNewJWK(t *testing.T) {
	secret, err := ioutil.ReadFile("./../testdata/mod_auth_jwt/secret_test_jws_ES512.key")
	if err != nil {
		t.Fatal(err)
	}
	keyMap := make(map[string]interface{})
	err = json.Unmarshal(secret, &keyMap)
	if err != nil {
		t.Fatal(err)
	}
	jwk, err := NewJWK(keyMap)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(jwk, jwk.Curve)
	if jwk.Kty != EC {
		t.Errorf("expected key type %d, got %d", EC, jwk.Kty)
	}
	if jwk.Curve.Crv != P521 {
		t.Errorf("expected crv value %d, got %d", P521, jwk.Curve.Crv)
	}
}
