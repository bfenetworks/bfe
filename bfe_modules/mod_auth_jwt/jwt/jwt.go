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

// Json Web Token
// see: https://tools.ietf.org/html/rfc7519
package jwt

import (
	"fmt"
	"strings"
	"time"
)

import (
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/config"
)

type tokenValidator interface {
	BasicCheck() error
}

type JWT struct {
	JWE     *JWE
	JWS     *JWS
	Claims  *Claims
	Nested  *JWT // nested jwt
	config  *config.AuthConfig
	context tokenValidator // current token context
}

// build token as JWS object
func (mJWT *JWT) buildJWS(token string) (err error) {
	mJWT.JWS, err = NewJWS(token, mJWT.config.Secret)
	if err != nil {
		return err
	}

	mJWT.context = mJWT.JWS
	mJWT.Claims, err = NewClaims(mJWT.JWS.Header.Decoded,
		mJWT.JWS.Payload.Decoded, mJWT.config.EnabledHeaderClaims)

	if mJWT.JWS.Header.Decoded["cty"] == "JWT" {
		// build nested JWT
		// error ignored because the jwk for the nested jwt may be different
		mJWT.Nested, _ = NewJWT(mJWT.JWS.Payload.Raw, mJWT.config)
	}

	return err
}

// build token as JWE object
func (mJWT *JWT) buildJWE(token string) (err error) {
	mJWT.JWE, err = NewJWE(token, mJWT.config.Secret)
	if err != nil {
		return err
	}

	mJWT.context = mJWT.JWE
	var payload map[string]interface{} = nil
	if mJWT.JWE.Payload != nil {
		payload = mJWT.JWE.Payload.Decoded
	}
	mJWT.Claims, err = NewClaims(mJWT.JWE.Header.Decoded, payload,
		mJWT.config.EnabledHeaderClaims)

	if mJWT.JWE.Header.Decoded["cty"] == "JWT" && mJWT.JWE.Payload != nil {
		// build nested JWT
		// error ignored because the jwk for the nested jwt may be different
		mJWT.Nested, _ = NewJWT(mJWT.JWE.Payload.Raw, mJWT.config)
	}

	return err
}

// exported validation interface
func (mJWT *JWT) Validate() (err error) {
	// perform basic check
	err = mJWT.context.BasicCheck()
	if err != nil {
		return err
	}

	// validate for claims
	err = mJWT.validateClaims()
	if err != nil {
		return err
	}

	if mJWT.Nested != nil && mJWT.config.ValidateNested {
		// validate for nested JWT
		return mJWT.Nested.Validate()
	}

	return nil
}

func (mJWT *JWT) validateClaims() (err error) {
	if err = mJWT.validateExp(); err != nil {
		return err
	}

	if err = mJWT.validateNbf(); err != nil {
		return err
	}

	if err = mJWT.validateIss(); err != nil {
		return err
	}

	if err = mJWT.validateAud(); err != nil {
		return err
	}

	if err = mJWT.validateSub(); err != nil {
		return err
	}

	return nil
}

func (mJWT *JWT) validateExp() (err error) {
	if !mJWT.config.ValidateClaimExp {
		return nil
	}

	claim, exp, ok := mJWT.Claims.Exp()
	if !ok {
		if claim == nil {
			// claim not present
			return nil
		}
		return fmt.Errorf("invalid value for exp claim: %s", claim)
	}

	if time.Now().After(time.Unix(exp, 0)) {
		return fmt.Errorf("the access token has been expired")
	}

	return nil
}

func (mJWT *JWT) validateNbf() (err error) {
	if !mJWT.config.ValidateClaimNbf {
		return nil
	}

	claim, nbf, ok := mJWT.Claims.Nbf()
	if !ok {
		if claim == nil {
			return nil
		}
		return fmt.Errorf("invalid value for nbf claim: %s", claim)
	}

	nbfTime := time.Unix(nbf, 0)
	if time.Now().Before(nbfTime) {
		return fmt.Errorf("this access token could not be accepted now, try again on %s", nbfTime)
	}

	return nil
}

func (mJWT *JWT) validateEqual(name string, target interface{}) (err error) {
	claim, ok := mJWT.Claims.Claim(name)
	if !ok {
		return nil
	}

	if claim != target {
		return fmt.Errorf("claim validation failed: %s", name)
	}

	return nil
}

func (mJWT *JWT) validateIss() (err error) {
	iss := mJWT.config.ValidateClaimIss
	if len(iss) == 0 {
		return nil
	}
	return mJWT.validateEqual("iss", iss)
}

func (mJWT *JWT) validateAud() (err error) {
	aud := mJWT.config.ValidateClaimAud
	if len(aud) == 0 {
		return nil
	}
	return mJWT.validateEqual("aud", aud)
}

func (mJWT *JWT) validateSub() (err error) {
	sub := mJWT.config.ValidateClaimSub
	if len(sub) == 0 {
		return nil
	}
	return mJWT.validateEqual("sub", sub)
}

func NewJWT(token string, conf *config.AuthConfig) (mJWT *JWT, err error) {
	mJWT, length := &JWT{config: conf}, len(strings.Split(token, "."))

	if length == 3 {
		err = mJWT.buildJWS(token)
	} else if length == 5 {
		err = mJWT.buildJWE(token)
	}

	if err != nil {
		return nil, err
	}

	return mJWT, nil
}
