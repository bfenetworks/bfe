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
	"crypto"
	"crypto/rsa"
	"errors"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"hash"
)

type PS struct {
	cSha crypto.Hash
	hSha hash.Hash
	pub  *rsa.PublicKey
}

func (ps *PS) Update(msg []byte) (n int, err error) {
	ps.hSha.Reset()
	return ps.hSha.Write(msg)
}

func (ps *PS) Sign() (sig []byte) {
	return ps.hSha.Sum(nil)
}

func (ps *PS) Verify(sig []byte) bool {
	return rsa.VerifyPSS(ps.pub, ps.cSha, ps.Sign(), sig, nil) == nil
}

func NewPS(sha crypto.Hash, mJWK *jwk.JWK) (ps JWSAlg, err error) {
	if mJWK.Kty != jwk.RSA {
		return nil, errors.New("unsupported algorithm type: PSx")
	}

	pub := &rsa.PublicKey{N: mJWK.RSA.N.Decoded, E: int(mJWK.RSA.E.Decoded.Uint64())}
	return &PS{sha, sha.New(), pub}, nil
}

func NewPS256(mJWK *jwk.JWK) (ps JWSAlg, err error) {
	return NewPS(crypto.SHA256, mJWK)
}

func NewPS384(mJWK *jwk.JWK) (ps JWSAlg, err error) {
	return NewPS(crypto.SHA384, mJWK)
}

func NewPS512(mJWK *jwk.JWK) (ps JWSAlg, err error) {
	return NewPS(crypto.SHA512, mJWK)
}
