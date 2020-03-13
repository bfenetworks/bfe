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
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
)

type AGCMKW struct {
	iv, tag []byte
	context cipher.AEAD
}

func (agk *AGCMKW) Decrypt(eCek []byte) (cek []byte, err error) {
	eCek = append(eCek, agk.tag...)
	return agk.context.Open(nil, agk.iv, eCek, []byte{})
}

func NewAGCMKW(kBit int, mJWK *jwk.JWK, header map[string]interface{}) (agk JWEAlg, err error) {
	if mJWK.Kty != jwk.OCT {
		return nil, fmt.Errorf("unsupported algorithm: A%dGCMKW", kBit)
	}

	if len(mJWK.Symmetric.K.Decoded) != kBit/8 {
		return nil, fmt.Errorf("bad key leangth for algorithm: A%dGCMKW", kBit)
	}

	params, err := ParseBase64URLHeader(header, true, "iv", "tag")
	if err != nil {
		return nil, err
	}

	iv, tag := params["iv"].Decoded, params["tag"].Decoded
	block, err := aes.NewCipher(mJWK.Symmetric.K.Decoded)
	if err != nil {
		return nil, err
	}

	context, err := cipher.NewGCMWithNonceSize(block, 12) // 96bit
	if err != nil {
		return nil, err
	}

	return &AGCMKW{iv, tag, context}, nil
}

func NewA128GCMKW(mJWK *jwk.JWK, header map[string]interface{}) (agk JWEAlg, err error) {
	return NewAGCMKW(128, mJWK, header)
}

func NewA192GCMKW(mJWK *jwk.JWK, header map[string]interface{}) (agk JWEAlg, err error) {
	return NewAGCMKW(192, mJWK, header)
}

func NewA256GCMKW(mJWK *jwk.JWK, header map[string]interface{}) (agk JWEAlg, err error) {
	return NewAGCMKW(256, mJWK, header)
}
