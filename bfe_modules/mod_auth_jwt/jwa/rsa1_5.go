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
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
)

type RSA15 struct {
	priv *rsa.PrivateKey
}

func (rsa15 *RSA15) Decrypt(eCek []byte) (cek []byte, err error) {
	return rsa.DecryptPKCS1v15(rand.Reader, rsa15.priv, eCek)
}

func NewRSA15(mJWK *jwk.JWK, _ map[string]interface{}) (rsa15 JWEAlg, err error) {
	if mJWK.Kty != jwk.RSA {
		return nil, fmt.Errorf("unsupported algorithm: RSA1_5")
	}
	return &RSA15{BuildRSAPrivateKey(mJWK)}, nil
}
