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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"testing"
)

func TestNewPBES2HS256A128KW(t *testing.T) {
	mJWK, _ := jwk.NewJWK(map[string]interface{}{
		"kty": "oct",
		"k": base64.RawURLEncoding.EncodeToString([]byte{84, 104, 117, 115, 32, 102, 114, 111, 109, 32, 109, 121, 32, 108,
			105, 112, 115, 44, 32, 98, 121, 32, 121, 111, 117, 114, 115, 44, 32,
			109, 121, 32, 115, 105, 110, 32, 105, 115, 32, 112, 117, 114, 103,
			101, 100, 46}),
	})
	header := `{
			"alg":"PBES2-HS256+A128KW",
			"p2s":"2WCTcJZ1Rvd_CJuJripQ1w",
			"p2c":4096,
			"enc":"A128CBC-HS256",
			"cty":"jwk+json"
	}`
	headerMap := make(map[string]interface{})
	if err := json.Unmarshal([]byte(header), &headerMap); err != nil {
		t.Fatal(err)
	}
	cek := []byte{111, 27, 25, 52, 66, 29, 20, 78, 92, 176, 56, 240, 65, 208, 82, 112,
		161, 131, 36, 55, 202, 236, 185, 172, 129, 23, 153, 194, 195, 48,
		253, 182}
	eCek := []byte{78, 186, 151, 59, 11, 141, 81, 240, 213, 245, 83, 211, 53, 188, 134,
		188, 66, 125, 36, 200, 222, 124, 5, 103, 249, 52, 117, 184, 140, 81,
		246, 158, 161, 177, 20, 33, 245, 57, 59, 4}
	context, err := NewPBES2HS256A128KW(mJWK, headerMap)
	if err != nil {
		t.Fatal(err)
	}
	res, err := context.Decrypt(eCek)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cek, res) {
		t.Error(res)
	}
}
