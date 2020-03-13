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
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"testing"
)

func TestA192GCMKW(t *testing.T) {
	eCek := []byte{254, 62, 144, 14, 60, 250, 236, 72, 31, 40, 28, 70, 237, 176, 240, 216}
	cek := []byte{249, 46, 197, 147, 69, 252, 219, 95, 168, 144, 184, 100, 131, 239, 56, 66}
	//iv := []byte{241, 217, 161, 182, 115, 115, 115, 100, 41, 206, 83, 78}
	//tag := []byte{128, 165, 239, 144, 167, 178, 178, 116, 224, 65, 94, 74, 8, 83, 208, 33}
	header := map[string]interface{}{
		"iv":  "8dmhtnNzc2QpzlNO",
		"tag": "gKXvkKeysnTgQV5KCFPQIQ",
	}
	mJWK, _ := jwk.NewJWK(map[string]interface{}{
		"kty": "oct",
		"k":   "X9QP8Nyk6n3360pIU_DDpOEEw3REmS4-",
	})
	context, err := NewA192GCMKW(mJWK, header)
	if err != nil {
		t.Fatal(err)
	}
	ret, err := context.Decrypt(eCek)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ret, cek) {
		t.Error(ret)
	}
}
