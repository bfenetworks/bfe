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

// defined item type for the key base64url-encoded and base64urlUInt-encoded
package jwk

import "math/big"

// base64url-encoded
type Base64URL struct {
	Raw     string
	Decoded []byte
}

// base64urlUInt-encoded
type Base64URLUint struct {
	Raw           string
	Decoded       *big.Int
	DecodedBase64 []byte
}

func NewBase64URL(raw string) (b *Base64URL, err error) {
	decoded, err := Base64URLDecode(raw)
	if err != nil {
		return nil, err
	}
	return &Base64URL{raw, decoded}, nil
}

func NewBase64URLUint(raw string) (b *Base64URLUint, err error) {
	oct, decoded, err := Base64URLUintDecode(raw)
	if err != nil {
		return nil, err
	}
	return &Base64URLUint{raw, decoded, oct}, nil
}
