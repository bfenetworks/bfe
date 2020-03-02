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
	EncryptedKey      *Base64URL
	InitialVector     *Base64URL
	CipherText        *Base64URL
	AuthenticationTag *Base64URL
	Secret            *jwk.JWK
}

func (mJWE *JWE) GetCek() (cek []byte, err error) {
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

func (mJWE *JWE) GetPayload() (payload []byte, err error) {
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
	cek, err := mJWE.GetCek()
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
	_, err = mJWE.GetPayload()
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
	return mJWE, nil
}
