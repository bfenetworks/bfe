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

package jwt

import (
	"encoding/base64"
	"encoding/json"
)

// base64url-encoded string
type Base64URLEncoded struct {
	Raw     string
	Decoded []byte
}

// base64url-encoded json object
type Base64URLEncodedJSON struct {
	Raw              string
	Decoded          map[string]interface{}
	DecodedBase64URL []byte
}

var (
	Base64URLDecode = base64.RawURLEncoding.DecodeString
)

func NewBase64URLEncoded(raw string) (b *Base64URLEncoded, err error) {
	decoded, err := Base64URLDecode(raw)
	if err != nil {
		return nil, err
	}

	return &Base64URLEncoded{raw, decoded}, nil
}

func NewBase64URLEncodedJSON(raw string, strict bool) (b *Base64URLEncodedJSON, err error) {
	// the parameter 'strict' tells whether json error should be report or not

	bDecoded, err := Base64URLDecode(raw)
	if err != nil {
		return nil, err
	}

	jMap := make(map[string]interface{})
	if err = json.Unmarshal(bDecoded, &jMap); err != nil {
		if strict {
			return nil, err
		}
		jMap = nil // in loose mode
	}

	return &Base64URLEncodedJSON{raw, jMap, bDecoded}, nil
}
