package jwa

import (
	"crypto"
	"crypto/rsa"
	"errors"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"hash"
)

type RS struct {
	cSha crypto.Hash
	hSha hash.Hash
	pub  *rsa.PublicKey
}

func (rs *RS) Update(msg []byte) (n int, err error) {
	rs.hSha.Reset()
	return rs.hSha.Write(msg)
}

func (rs *RS) Sign() (sig []byte) {
	return rs.hSha.Sum(nil)
}

func (rs *RS) Verify(sig []byte) bool {
	if rsa.VerifyPKCS1v15(rs.pub, rs.cSha, rs.Sign(), sig) != nil {
		return false
	}
	return true
}

func NewRS(sha crypto.Hash, mJWK *jwk.JWK) (rs SignAlg, err error) {
	if mJWK.Kty != jwk.RSA {
		return nil, errors.New("unsupported algorithm type: RSx")
	}
	return &RS{sha, sha.New(), &rsa.PublicKey{N: mJWK.RSA.N.Decoded, E: int(mJWK.RSA.E.Decoded.Uint64())}}, nil
}

func NewRS256(mJWK *jwk.JWK) (rs SignAlg, err error) {
	return NewRS(crypto.SHA256, mJWK)
}

func NewRS384(mJWK *jwk.JWK) (rs SignAlg, err error) {
	return NewRS(crypto.SHA384, mJWK)
}

func NewRS512(mJWK *jwk.JWK) (rs SignAlg, err error) {
	return NewRS(crypto.SHA512, mJWK)
}
