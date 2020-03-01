package jwa

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"errors"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"hash"
	"math/big"
)

type ES struct {
	sha hash.Hash
	pub *ecdsa.PublicKey
}

func (es *ES) Update(msg []byte) (n int, err error) {
	es.sha.Reset()
	return es.sha.Write(msg)
}

func (es *ES) Sign() (sig []byte) {
	return es.sha.Sum(nil)
}

func (es *ES) Verify(sig []byte) bool {
	r := new(big.Int).SetBytes(sig[:len(sig)/2])
	s := new(big.Int).SetBytes(sig[len(sig)/2:])
	if !ecdsa.Verify(es.pub, es.Sign(), r, s) {
		return false
	}
	return true
}

func NewES(sha crypto.Hash, curve elliptic.Curve, mJWK *jwk.JWK) (es SignAlg, err error) {
	if mJWK.Kty != jwk.EC {
		return nil, errors.New("unsupported algorithm type: ESx")
	}
	return &ES{sha.New(), &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(mJWK.Curve.X.Decoded), Y: new(big.Int).SetBytes(mJWK.Curve.Y.Decoded)}}, nil
}

func NewES256(mJWK *jwk.JWK) (es SignAlg, err error) {
	if mJWK.Curve.Crv != jwk.P256 {
		return nil, errors.New("unsupported algorithm type: ESx")
	}
	return NewES(crypto.SHA256, elliptic.P256(), mJWK)
}

func NewES384(mJWK *jwk.JWK) (es SignAlg, err error) {
	if mJWK.Curve.Crv != jwk.P384 {
		return nil, errors.New("unsupported algorithm type: ESx")
	}
	return NewES(crypto.SHA384, elliptic.P384(), mJWK)
}

func NewES512(mJWK *jwk.JWK) (es SignAlg, err error) {
	if mJWK.Curve.Crv != jwk.P521 {
		return nil, errors.New("unsupported algorithm type: ESx")
	}
	return NewES(crypto.SHA512, elliptic.P521(), mJWK)
}
