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
)

type AGCM struct {
	context cipher.AEAD
}

func (agcm *AGCM) Decrypt(iv, aad, cipherText, tag []byte) (msg []byte, err error) {
	cipherText = append(cipherText, tag...)
	return agcm.context.Open(nil, iv, cipherText, aad)
}

func NewAGCM(kBit int, cek []byte) (agcm JWEEnc, err error) {
	if kBit/8 != len(cek) {
		return nil, fmt.Errorf("invalid CEK length for A%dGCM: %d", kBit, len(cek))
	}

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return &AGCM{gcm}, nil
}

func NewA128GCM(cek []byte) (agcm JWEEnc, err error) {
	return NewAGCM(128, cek)
}

func NewA192GCM(cek []byte) (agcm JWEEnc, err error) {
	return NewAGCM(192, cek)
}

func NewA256GCM(cek []byte) (agcm JWEEnc, err error) {
	return NewAGCM(256, cek)
}
