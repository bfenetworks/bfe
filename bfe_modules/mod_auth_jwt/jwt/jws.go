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

// Json Web signature
// see: https://tools.ietf.org/html/rfc7515
package jwt

import (
	"fmt"
	"strings"
)

import (
	"gopkg.in/square/go-jose.v2"
)

type JWS struct {
	Raw       string
	Header    *Base64URLEncodedJSON
	Payload   *Base64URLEncodedJSON
	Signature *Base64URLEncoded
	Secret    *jose.JSONWebKey
}

func (mJWS *JWS) BasicCheck() (err error) {
	sig, err := jose.ParseSigned(mJWS.Raw)
	if err != nil {
		return err
	}

	var key = mJWS.Secret.Key
	if _, ok := key.([]byte); !ok {
		key = mJWS.Secret.Public()
	}

	_, err = sig.Verify(key)

	return err
}

func NewJWS(token string, secret *jose.JSONWebKey) (mJWS *JWS, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("not a JWS token: %s", token)
	}

	mJWS = &JWS{Raw: token, Secret: secret}
	mJWS.Header, err = NewBase64URLEncodedJSON(parts[0], true)
	if err != nil {
		return nil, err
	}

	// do not report json error
	// it may be limited to the header parameter 'cty'
	mJWS.Payload, err = NewBase64URLEncodedJSON(parts[1], false)
	if err != nil {
		return nil, err
	}

	mJWS.Signature, err = NewBase64URLEncoded(parts[2])
	if err != nil {
		return nil, err
	}

	return mJWS, nil
}
