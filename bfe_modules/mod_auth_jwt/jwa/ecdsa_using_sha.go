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
	"crypto/ecdsa"
	"crypto/elliptic"
	"errors"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"hash"
	"math/big"
)

type ES struct {
	sha hash.Hash
	pub *ecdsa.PublicKey
}

func (es *ES) Update(msg []byte) (n int, err error) {
	es.sha.Reset()
	return es.sha.Write(msg)
}

func (es *ES) Sign() (sig []byte) {
	return es.sha.Sum(nil)
}

func (es *ES) Verify(sig []byte) bool {
	r := new(big.Int).SetBytes(sig[:len(sig)/2])
	s := new(big.Int).SetBytes(sig[len(sig)/2:])
	return ecdsa.Verify(es.pub, es.Sign(), r, s)
}

func NewES(sha crypto.Hash, curve elliptic.Curve, mJWK *jwk.JWK) (es JWSAlg, err error) {
	if mJWK.Kty != jwk.EC {
		return nil, errors.New("unsupported algorithm type: ESx")
	}

	pub := &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(mJWK.Curve.X.Decoded),
		Y:     new(big.Int).SetBytes(mJWK.Curve.Y.Decoded),
	}

	return &ES{sha.New(), pub}, nil
}

func NewES256(mJWK *jwk.JWK) (es JWSAlg, err error) {
	if mJWK.Curve.Crv != jwk.P256 {
		return nil, errors.New("unsupported algorithm: ES256")
	}
	return NewES(crypto.SHA256, elliptic.P256(), mJWK)
}

func NewES384(mJWK *jwk.JWK) (es JWSAlg, err error) {
	if mJWK.Curve.Crv != jwk.P384 {
		return nil, errors.New("unsupported algorithm: ES384")
	}
	return NewES(crypto.SHA384, elliptic.P384(), mJWK)
}

func NewES512(mJWK *jwk.JWK) (es JWSAlg, err error) {
	if mJWK.Curve.Crv != jwk.P521 {
		return nil, errors.New("unsupported algorithm: ES512")
	}
	return NewES(crypto.SHA512, elliptic.P521(), mJWK)
}
