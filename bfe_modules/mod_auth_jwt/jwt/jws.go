// Json Web signature
// see: https://tools.ietf.org/html/rfc7515

package jwt

import (
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwa"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"strings"
)

type JWS struct {
	Raw       string
	Header    *Base64URLJson
	Payload   *Base64URLJson
	Signature *Base64URL
	Secret    *jwk.JWK
}

func (mJWS *JWS) checkSignature(handler jwa.JWSAlg) (err error) {
	msg := []byte(strings.Join(strings.Split(mJWS.Raw, ".")[:2], "."))
	_, err = handler.Update(msg)
	if err != nil {
		return err
	}
	if !handler.Verify(mJWS.Signature.Decoded) {
		return fmt.Errorf("JWT signature check failed")
	}
	return nil
}

func (mJWS *JWS) BasicCheck() (err error) {
	alg, ok := mJWS.Header.Decoded["alg"]
	if !ok {
		return fmt.Errorf("missing header parameter alg")
	}
	algStr, ok := alg.(string)
	if !ok {
		return fmt.Errorf("invalid value for header parameter alg: %+v", alg)
	}
	// get factory function by alg
	algFactory, ok := jwa.JWSAlgSet[algStr]
	if !ok {
		return fmt.Errorf("unknown alg: %s", algStr)
	}
	// create handler
	context, err := algFactory(mJWS.Secret)
	if err != nil {
		return err
	}
	return mJWS.checkSignature(context)
}

func NewJWS(token string, secret *jwk.JWK) (mJWS *JWS, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("not a JWS token: %s", token)
	}
	mJWS = &JWS{Raw: token, Secret: secret}
	mJWS.Header, err = NewBase64URLJson(parts[0], true)
	if err != nil {
		return nil, err
	}
	// do not report json error
	// it may be limited to the header parameter 'cty'
	mJWS.Payload, err = NewBase64URLJson(parts[1], false)
	if err != nil {
		return nil, err
	}
	mJWS.Signature, err = NewBase64URL(parts[2])
	if err != nil {
		return nil, err
	}
	return mJWS, nil
}
