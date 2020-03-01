package jwa

import (
	"crypto"
	"crypto/rsa"
	"errors"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"hash"
)

type PS struct {
	cSha crypto.Hash
	hSha hash.Hash
	pub  *rsa.PublicKey
}

func (ps *PS) Update(msg []byte) (n int, err error) {
	ps.hSha.Reset()
	return ps.hSha.Write(msg)
}

func (ps *PS) Sign() (sig []byte) {
	return ps.hSha.Sum(nil)
}

func (ps *PS) Verify(sig []byte) bool {
	return rsa.VerifyPSS(ps.pub, ps.cSha, ps.Sign(), sig, nil) == nil
}

func NewPS(sha crypto.Hash, mJWK *jwk.JWK) (ps JWSAlg, err error) {
	if mJWK.Kty != jwk.RSA {
		return nil, errors.New("unsupported algorithm type: PSx")
	}
	pub := &rsa.PublicKey{N: mJWK.RSA.N.Decoded, E: int(mJWK.RSA.E.Decoded.Uint64())}
	return &PS{sha, sha.New(), pub}, nil
}

func NewPS256(mJWK *jwk.JWK) (ps JWSAlg, err error) {
	return NewPS(crypto.SHA256, mJWK)
}

func NewPS384(mJWK *jwk.JWK) (ps JWSAlg, err error) {
	return NewPS(crypto.SHA384, mJWK)
}

func NewPS512(mJWK *jwk.JWK) (ps JWSAlg, err error) {
	return NewPS(crypto.SHA512, mJWK)
}
