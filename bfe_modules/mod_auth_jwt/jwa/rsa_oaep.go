package jwa

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"hash"
)

type RSAOAEP struct {
	priv *rsa.PrivateKey
	hash hash.Hash
}

func (ro *RSAOAEP) Decrypt(eCek []byte) (cek []byte, err error) {
	return rsa.DecryptOAEP(ro.hash, rand.Reader, ro.priv, eCek, nil)
}

func NewRSAOAEP(hash hash.Hash, mJWK *jwk.JWK) (ro JWEAlg, err error) {
	if mJWK.Kty != jwk.RSA {
		return nil, fmt.Errorf("unsupported algorithm: RSAOEAPx")
	}
	return &RSAOAEP{BuildRSAPrivateKey(mJWK), hash}, nil
}

func NewRSAOAEPDefault(mJWK *jwk.JWK, _ map[string]interface{}) (ro JWEAlg, err error) {
	return NewRSAOAEP(sha1.New(), mJWK)
}

func NewRSAOAEP256(mJWK *jwk.JWK, _ map[string]interface{}) (ro JWEAlg, err error) {
	return NewRSAOAEP(sha256.New(), mJWK)
}
