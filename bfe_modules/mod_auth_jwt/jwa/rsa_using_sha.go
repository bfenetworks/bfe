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

type RS struct {
	cSha crypto.Hash
	hSha hash.Hash
	pub  *rsa.PublicKey
}

func (rs *RS) Update(msg []byte) (n int, err error) {
	rs.hSha.Reset()
	return rs.hSha.Write(msg)
}

func (rs *RS) Sign() (sig []byte) {
	return rs.hSha.Sum(nil)
}

func (rs *RS) Verify(sig []byte) bool {
	return rsa.VerifyPKCS1v15(rs.pub, rs.cSha, rs.Sign(), sig) == nil
}

func NewRS(sha crypto.Hash, mJWK *jwk.JWK) (rs JWSAlg, err error) {
	if mJWK.Kty != jwk.RSA {
		return nil, errors.New("unsupported algorithm type: RSx")
	}

	pub := &rsa.PublicKey{N: mJWK.RSA.N.Decoded, E: int(mJWK.RSA.E.Decoded.Uint64())}
	return &RS{sha, sha.New(), pub}, nil
}

func NewRS256(mJWK *jwk.JWK) (rs JWSAlg, err error) {
	return NewRS(crypto.SHA256, mJWK)
}

func NewRS384(mJWK *jwk.JWK) (rs JWSAlg, err error) {
	return NewRS(crypto.SHA384, mJWK)
}

func NewRS512(mJWK *jwk.JWK) (rs JWSAlg, err error) {
	return NewRS(crypto.SHA512, mJWK)
}
