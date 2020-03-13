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
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"hash"
)

type HS struct {
	context hash.Hash
}

func (hs *HS) Update(msg []byte) (n int, err error) {
	hs.context.Reset()
	return hs.context.Write(msg)
}

func (hs *HS) Sign() (sig []byte) {
	return hs.context.Sum(nil)
}

func (hs *HS) Verify(sig []byte) bool {
	return hmac.Equal(sig, hs.Sign())
}

func NewHS(factory HashFactory, mJWK *jwk.JWK) (hs JWSAlg, err error) {
	if mJWK.Kty != jwk.OCT {
		return nil, errors.New("unsupported algorithm type: HSx")
	}
	return &HS{hmac.New(factory, mJWK.Symmetric.K.Decoded)}, nil
}

func NewHS256(mJWK *jwk.JWK) (hs JWSAlg, err error) {
	return NewHS(sha256.New, mJWK)
}

func NewHS384(mJWK *jwk.JWK) (hs JWSAlg, err error) {
	return NewHS(sha512.New384, mJWK)
}

func NewHS512(mJWK *jwk.JWK) (hs JWSAlg, err error) {
	return NewHS(sha512.New, mJWK)
}
