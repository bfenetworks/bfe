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
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
)

type Dir struct {
	cek []byte
}

func (dir *Dir) Decrypt(_ []byte) (cek []byte, err error) {
	return dir.cek, nil
}

func NewDir(mJWK *jwk.JWK, _ map[string]interface{}) (dir JWEAlg, err error) {
	if mJWK.Kty != jwk.OCT {
		return nil, fmt.Errorf("unsupported algorithm: dir")
	}
	return &Dir{mJWK.Symmetric.K.Decoded}, nil
}
