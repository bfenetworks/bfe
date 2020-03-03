package jwa

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"golang.org/x/crypto/pbkdf2"
)

type PBESHSAKW struct {
	wrapper   jweAlgFactory
	jwk       *jwk.JWK
	factory   HashFactory
	saltInput []byte
	count     int
	kBit      int
}

func (phakw *PBESHSAKW) unwrap(key, eCek []byte) (cek []byte, err error) {
	mJWK, err := jwk.NewJWK(map[string]interface{}{
		"kty": "oct",
		"k":   base64.RawURLEncoding.EncodeToString(key),
	})
	if err != nil {
		return nil, err
	}
	context, err := phakw.wrapper(mJWK, nil)
	if err != nil {
		return nil, err
	}
	return context.Decrypt(eCek)
}

func (phakw *PBESHSAKW) Decrypt(eCek []byte) (cek []byte, err error) {
	dk := pbkdf2.Key(phakw.jwk.Symmetric.K.Decoded, phakw.saltInput, phakw.count, phakw.kBit/8, phakw.factory)
	if phakw.wrapper != nil {
		return phakw.unwrap(dk, eCek)
	}
	return cek, nil
}

func NewPBES2HSAKW(wrapper jweAlgFactory, kBit int, factory HashFactory, mJWK *jwk.JWK, header map[string]interface{}) (phakw JWEAlg, err error) {
	if mJWK.Kty != jwk.OCT {
		return nil, fmt.Errorf("unsupported algorithm: PBES2-HS%d+A%dKW", factory().Size(), kBit)
	}
	params, err := ParseBase64URLHeader(header, false, "p2s")
	if err != nil {
		return nil, err
	}
	alg, p2s, p2c := []byte(header["alg"].(string)), params["p2s"].Decoded, header["p2c"].(float64)
	//The salt value used is (UTF8(Alg) || 0x00 || Salt Input)
	//Alg is the "alg" (algorithm) Header Parameter value.
	saltInput := StitchingSalt(alg, []byte{0}, p2s)
	count := int(p2c)
	return &PBESHSAKW{wrapper, mJWK, factory, saltInput, count, kBit}, nil
}

func NewPBES2HS256A128KW(mJWK *jwk.JWK, header map[string]interface{}) (phakw JWEAlg, err error) {
	return NewPBES2HSAKW(NewA128KW, 128, sha256.New, mJWK, header)
}

func NewPBES2HS384A192KW(mJWK *jwk.JWK, header map[string]interface{}) (phakw JWEAlg, err error) {
	return NewPBES2HSAKW(NewA192KW, 192, sha512.New384, mJWK, header)
}

func NewPBES2HS512A256KW(mJWK *jwk.JWK, header map[string]interface{}) (phakw JWEAlg, err error) {
	return NewPBES2HSAKW(NewA256KW, 256, sha512.New, mJWK, header)
}
