package jwa

import (
	"crypto/rsa"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"hash"
	"math/big"
	"strings"
)

type HashFactory func() hash.Hash

func BuildRSACRTValues(mJWK *jwk.JWK) (crtv []rsa.CRTValue) {
	oth := mJWK.RSA.Oth
	if len(oth) == 0 {
		return nil
	}
	crtv = make([]rsa.CRTValue, len(oth))
	for _, o := range oth {
		crt := rsa.CRTValue{
			Exp:   o.D.Decoded,
			Coeff: o.T.Decoded,
			R:     o.R.Decoded,
		}
		crtv = append(crtv, crt)
	}
	return crtv
}

func BuildRSAPrecomputed(mJWK *jwk.JWK) (precomputed *rsa.PrecomputedValues) {
	if !mJWK.RSA.Full {
		return nil
	}
	return &rsa.PrecomputedValues{
		Dp:        mJWK.RSA.DP.Decoded,
		Dq:        mJWK.RSA.DQ.Decoded,
		Qinv:      mJWK.RSA.QI.Decoded,
		CRTValues: BuildRSACRTValues(mJWK),
	}
}

func BuildRSAPrimes(mJWK *jwk.JWK) (primes []*big.Int) {
	primes = []*big.Int{mJWK.RSA.P.Decoded, mJWK.RSA.Q.Decoded}
	for _, o := range mJWK.RSA.Oth {
		primes = append(primes, o.R.Decoded)
	}
	return primes
}

func BuildRSAPublicKey(mJWK *jwk.JWK) (pub *rsa.PublicKey) {
	return &rsa.PublicKey{
		N: mJWK.RSA.N.Decoded,
		E: int(mJWK.RSA.E.Decoded.Uint64()),
	}
}

func BuildRSAPrivateKey(mJWK *jwk.JWK) (priv *rsa.PrivateKey) {
	var precomputed rsa.PrecomputedValues
	precomputedPtr := BuildRSAPrecomputed(mJWK)
	if precomputedPtr != nil {
		precomputed = *precomputedPtr
	}
	return &rsa.PrivateKey{
		PublicKey:   *BuildRSAPublicKey(mJWK),
		D:           mJWK.RSA.D.Decoded,
		Primes:      BuildRSAPrimes(mJWK),
		Precomputed: precomputed,
	}
}

// in the case that package crypto caused panic,
// set error to the return value instead of panic
func CatchCryptoPanic(errPtr *error) {
	recovered := recover()
	if recovered == nil {
		return
	}
	err := fmt.Errorf("%s", recovered)
	if !strings.Contains(err.Error(), "crypto") {
		panic(err) // other exception
	}
	*errPtr = err // this pointer was bind to the return value
}
