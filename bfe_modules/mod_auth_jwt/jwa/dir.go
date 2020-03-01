package jwa

import (
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
)

type Dir struct {
	cek []byte
}

func (dir *Dir) Decrypt(_ []byte) (cek []byte, err error) {
	return dir.cek, nil
}

func NewDir(mJWK *jwk.JWK, _ map[string]interface{}) (dir JWEAlg, err error) {
	if mJWK.Kty != jwk.OCT {
		return nil, fmt.Errorf("unsupported algorithm: dir")
	}
	return &Dir{mJWK.Symmetric.K.Decoded}, nil
}
