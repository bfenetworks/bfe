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

// Json Web Encryption
// see: https://tools.ietf.org/html/rfc7516
package jwt

import (
	"fmt"
	"strings"
)

import (
	"gopkg.in/square/go-jose.v2"
)

type JWE struct {
	Raw               string
	Header            *Base64URLEncodedJSON
	Payload           *Base64URLEncodedJSON
	EncryptedKey      *Base64URLEncoded
	InitialVector     *Base64URLEncoded
	CipherText        *Base64URLEncoded
	AuthenticationTag *Base64URLEncoded
	Secret            *jose.JSONWebKey
}

func (mJWE *JWE) Plaintext() (plaintext []byte, err error) {
	enc, err := jose.ParseEncrypted(mJWE.Raw)
	if err != nil {
		return nil, err
	}

	return enc.Decrypt(mJWE.Secret)
}

func (mJWE *JWE) BasicCheck() (err error) {
	_, err = mJWE.Plaintext()

	return err
}

func NewJWE(token string, secret *jose.JSONWebKey) (mJWE *JWE, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 5 {
		return nil, fmt.Errorf("not a JWE token: %s", token)
	}

	mJWE = &JWE{Raw: token, Secret: secret}
	mJWE.Header, err = NewBase64URLEncodedJSON(parts[0], true)
	if err != nil {
		return nil, err
	}

	mJWE.EncryptedKey, err = NewBase64URLEncoded(parts[1])
	if err != nil {
		return nil, err
	}

	mJWE.InitialVector, err = NewBase64URLEncoded(parts[2])
	if err != nil {
		return nil, err
	}

	mJWE.CipherText, err = NewBase64URLEncoded(parts[3])
	if err != nil {
		return nil, err
	}

	mJWE.AuthenticationTag, err = NewBase64URLEncoded(parts[4])
	if err != nil {
		return nil, err
	}

	// parse payload for lookup claims or nested JWT
	// error ignored parsing payload this stage
	plaintext, err := mJWE.Plaintext()
	if err == nil {
		// payload can be not a base64URL-encoded json object
		mJWE.Payload, _ = NewBase64URLEncodedJSON(string(plaintext), false)
	}

	return mJWE, nil
}
