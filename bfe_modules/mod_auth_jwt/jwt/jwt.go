// Json Web Token
// see: https://tools.ietf.org/html/rfc7519

package jwt

import (
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"strings"
	"time"
)

type tokenValidator interface {
	BasicCheck() error
}

// universal parameters for module Config and product Config
type Config struct {
	Secret               *jwk.JWK
	SecretPath           string
	EnabledPayloadClaims bool
	ValidateNested       bool
	ValidateClaimExp     bool
	ValidateClaimNbf     bool
	ValidateClaimIss     string
	ValidateClaimSub     string
	ValidateClaimAud     string
}

type JWT struct {
	JWE     *JWE
	JWS     *JWS
	Claims  *Claims
	Nested  *JWT // nested jwt
	config  *Config
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
		mJWT.JWS.Payload.Decoded, mJWT.config.EnabledPayloadClaims)
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
	mJWT.Claims, err = NewClaims(mJWT.JWE.Header.Decoded, nil,
		mJWT.config.EnabledPayloadClaims)
	if mJWT.JWE.Header.Decoded["cty"] == "JWT" {
		// build nested JWT
		// error ignored because the jwk for the nested jwt may be different
		token, _ := mJWT.JWE.GetPayload()
		mJWT.Nested, _ = NewJWT(string(token), mJWT.config)
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
		return fmt.Errorf("this access token could not be accept now, try again on %s", nbfTime)
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
	return mJWT.validateEqual("iss", aud)
}

func (mJWT *JWT) validateSub() (err error) {
	sub := mJWT.config.ValidateClaimIss
	if len(sub) == 0 {
		return nil
	}
	return mJWT.validateEqual("iss", sub)
}

func NewJWT(token string, conf *Config) (mJWT *JWT, err error) {
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
