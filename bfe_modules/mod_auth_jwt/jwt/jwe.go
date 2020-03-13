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
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwa"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"strings"
)

type JWE struct {
	Raw               string
	Header            *Base64URLJson
	Payload           *Base64URLJson
	EncryptedKey      *Base64URL
	InitialVector     *Base64URL
	CipherText        *Base64URL
	AuthenticationTag *Base64URL
	Secret            *jwk.JWK
}

func (mJWE *JWE) Cek() (cek []byte, err error) {
	alg, ok := mJWE.Header.Decoded["alg"]
	if !ok {
		return nil, fmt.Errorf("missing header parameter alg")
	}

	algStr, ok := alg.(string)
	if !ok {
		return nil, fmt.Errorf("invalid value for header parameter alg: %+v", alg)
	}

	algFactory, ok := jwa.JWEAlgSet[algStr]
	if !ok {
		return nil, fmt.Errorf("unknown alg: %s", algStr)
	}

	context, err := algFactory(mJWE.Secret, mJWE.Header.Decoded)
	if err != nil {
		return nil, err
	}

	return context.Decrypt(mJWE.EncryptedKey.Decoded)
}

func (mJWE *JWE) Plaintext() (plaintext []byte, err error) {
	enc, ok := mJWE.Header.Decoded["enc"]
	if !ok {
		return nil, fmt.Errorf("missing header parameter enc")
	}

	encStr, ok := enc.(string)
	if !ok {
		return nil, fmt.Errorf("invalid value for header parameter enc: %+v", enc)
	}

	encFactory, ok := jwa.JWEEncSet[encStr]
	if !ok {
		return nil, fmt.Errorf("unknown enc: %s", encStr)
	}

	cek, err := mJWE.Cek()
	if err != nil {
		return nil, err
	}

	context, err := encFactory(cek)
	if err != nil {
		return nil, err
	}

	return context.Decrypt(mJWE.InitialVector.Decoded, []byte(mJWE.Header.Raw),
		mJWE.CipherText.Decoded, mJWE.AuthenticationTag.Decoded)
}

func (mJWE *JWE) BasicCheck() (err error) {
	_, err = mJWE.Plaintext()
	return err
}

func NewJWE(token string, secret *jwk.JWK) (mJWE *JWE, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 5 {
		return nil, fmt.Errorf("not a JWE token: %s", token)
	}

	mJWE = &JWE{Raw: token, Secret: secret}
	mJWE.Header, err = NewBase64URLJson(parts[0], true)
	if err != nil {
		return nil, err
	}

	mJWE.EncryptedKey, err = NewBase64URL(parts[1])
	if err != nil {
		return nil, err
	}

	mJWE.InitialVector, err = NewBase64URL(parts[2])
	if err != nil {
		return nil, err
	}

	mJWE.CipherText, err = NewBase64URL(parts[3])
	if err != nil {
		return nil, err
	}

	mJWE.AuthenticationTag, err = NewBase64URL(parts[4])
	if err != nil {
		return nil, err
	}

	// parse payload for lookup claims or nested JWT
	// error ignored parsing payload this stage
	plaintext, err := mJWE.Plaintext()
	if err == nil {
		// payload can be not a base64URL-encoded json object
		mJWE.Payload, _ = NewBase64URLJson(string(plaintext), false)
	}

	return mJWE, nil
}
