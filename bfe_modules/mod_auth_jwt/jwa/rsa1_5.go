package jwa

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
)

type RSA15 struct {
	priv *rsa.PrivateKey
}

func (rsa15 *RSA15) Decrypt(eCek []byte) (cek []byte, err error) {
	return rsa.DecryptPKCS1v15(rand.Reader, rsa15.priv, eCek)
}

func NewRSA15(mJWK *jwk.JWK, _ map[string]interface{}) (rsa15 JWEAlg, err error) {
	if mJWK.Kty != jwk.RSA {
		return nil, fmt.Errorf("unsupported algorithm: RSA1_5")
	}
	return &RSA15{BuildRSAPrivateKey(mJWK)}, nil
}
