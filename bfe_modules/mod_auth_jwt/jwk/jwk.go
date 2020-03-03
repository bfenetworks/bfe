// jwk entry
package jwk

import (
	"fmt"
)

// JWK parameters
// see: https://tools.ietf.org/html/rfc7518
type JWK struct {
	Kty       int              // key type
	Curve     *curveParams     // curve
	Symmetric *symmetricParams // symmetric
	RSA       *rsaParams       // rsa
}

const (
	OCT = iota
	EC
	RSA
)

const (
	P256 = iota
	P384
	P521
)

var crvMapping = map[string]int{
	"P-256": P256,
	"P-384": P384,
	"P-521": P521,
}

func GetCrvCode(crv string) (code int, ok bool) {
	code, ok = crvMapping[crv]
	return code, ok
}

func NewJWK(keyMap map[string]interface{}) (mJWK *JWK, err error) {
	mJWK, kty := new(JWK), keyMap["kty"]

	switch kty {

	case "oct":
		mJWK.Kty = OCT
		mJWK.Symmetric, err = buildSymmetricParams(keyMap)

	case "EC":
		mJWK.Kty = EC
		mJWK.Curve, err = buildCurveParams(keyMap, true)

	case "RSA":
		mJWK.Kty = RSA
		mJWK.RSA, err = buildRSAParams(keyMap, true)

	default:
		return nil, fmt.Errorf("invalid key value for kty: %+v", kty)

	}

	if err != nil {
		return nil, err
	}

	return mJWK, nil
}

func NewJWKPub(keyMap map[string]interface{}) (mJWK *JWK, err error) {
	mJWK, kty := new(JWK), keyMap["kty"]

	switch kty {

	case "EC":
		mJWK.Kty = EC
		mJWK.Curve, err = buildCurveParams(keyMap, false)

	case "RSA":
		mJWK.Kty = RSA
		mJWK.RSA, err = buildRSAParams(keyMap, false)

	default:
		return nil, fmt.Errorf("invalid key value for kty(public): %+v", kty)

	}

	if err != nil {
		return nil, err
	}

	return mJWK, nil
}
