// Copyright (c) 2019 Baidu, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

func ParseBase64URLHeader(header map[string]interface{}, required bool, key ...string) (result map[string]*jwk.Base64URL, err error) {
	result = make(map[string]*jwk.Base64URL, len(key))
	for _, k := range key {
		v, ok := header[k]
		if !ok {
			if required {
				return nil, fmt.Errorf("missing header parameter: %s", k)
			}
			v = ""
		}

		vStr, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("bad value type for header parameter: %s", k)
		}

		result[k], err = jwk.NewBase64URL(vStr)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func StitchingSalt(arrays ...[]byte) []byte {
	var ret = arrays[0]
	for _, arr := range arrays[1:] {
		ret = append(ret, arr...)
	}
	return ret
}
