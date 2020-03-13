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

// Concatenation Key Derivation Function
// see: https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-56Ar2.pdf
package jwa

import (
	"encoding/binary"
	"fmt"
	"hash"
	"math"
)

type ConcatKDF struct {
	H hash.Hash
}

func (kdf *ConcatKDF) Derive(z []byte, keyDataLen int, otherInfo []byte) (key []byte, err error) {
	if keyDataLen*kdf.H.Size()*8 > math.MaxInt32 {
		return nil, fmt.Errorf("invalid key data length: %d", keyDataLen)
	}

	cByte := make([]byte, 4)                                 // container for uint32
	kLen, hLen := uint32(keyDataLen), uint32(kdf.H.Size()*8) // bit length
	var outLen, counter uint32 = 0, 1

	// calculate hash value for the input: counter(32bit) + key_material + otherinfo
	// repeat this util got keyDataLen bits of hash value (with counter increment)
	for outLen < kLen {
		binary.BigEndian.PutUint32(cByte, counter)
		kdf.H.Reset()
		kdf.H.Write(cByte)
		kdf.H.Write(z)
		kdf.H.Write(otherInfo)
		key = append(key, kdf.H.Sum(nil)...)
		outLen += hLen
		counter++
	}

	return key[:kLen/8], nil // the first kLen bits of key
}

func NewConcatKDF(h hash.Hash) *ConcatKDF {
	return &ConcatKDF{h}
}
