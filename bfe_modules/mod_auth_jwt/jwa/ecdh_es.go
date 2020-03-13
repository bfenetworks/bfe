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

// Key Agreement with Elliptic Curve Diffie-Hellman Ephemeral Static
// see: https://tools.ietf.org/html/rfc7518#section-4.6
package jwa

import (
	"crypto"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"math/big"
)

type ECDHES struct {
	priv    *jwk.JWK
	pub     *jwk.JWK
	wrapper jweAlgFactory
	kBit    int
	other   []byte
}

func (ec *ECDHES) Curve() elliptic.Curve {
	switch ec.priv.Curve.Crv {
	case jwk.P256:
		return elliptic.P256()
	case jwk.P384:
		return elliptic.P384()
	case jwk.P521:
		return elliptic.P521()
	}
	return nil
}

func (ec *ECDHES) SharedKey() (key []byte) {
	x := new(big.Int).SetBytes(ec.pub.Curve.X.Decoded)
	y := new(big.Int).SetBytes(ec.pub.Curve.Y.Decoded)
	k, _ := ec.Curve().ScalarMult(x, y, ec.priv.Curve.D.Decoded)
	return k.Bytes()
}

func (ec *ECDHES) unwrap(key, eCek []byte) (cek []byte, err error) {
	mJWK, err := jwk.NewJWK(map[string]interface{}{
		"kty": "oct",
		"k":   base64.RawURLEncoding.EncodeToString(key),
	})
	if err != nil {
		return nil, err
	}

	context, err := ec.wrapper(mJWK, nil)
	if err != nil {
		return nil, err
	}

	return context.Decrypt(eCek)
}

func (ec *ECDHES) Decrypt(eCek []byte) (cek []byte, err error) {
	kdf := NewConcatKDF(crypto.SHA256.New())
	cek, err = kdf.Derive(ec.SharedKey(), ec.kBit, ec.other)
	if err != nil {
		return nil, err
	}

	if ec.wrapper != nil {
		// unwrap encrypted Key (eCek) using derived key (cek) as symmetric key
		return ec.unwrap(cek, eCek)
	}

	return cek, nil
}

func otherInfo(alg, apu, apv []byte, kBit int) (other []byte) {
	// see chapter 5.8.1.2: https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-56Ar2.pdf
	// see also: https://tools.ietf.org/html/rfc7518#section-4.6.2
	//
	// For this format, OtherInfo is a bit string equal to the following concatenation:
	// AlgorithmID || PartyUInfo || PartyVInfo {|| SuppPubInfo }{|| SuppPrivInfo },
	// where the five subfields are bit strings comprised of items of information as described in Section 5.8.1.2.

	temp := make([]byte, 4) // 32 bit container

	binary.BigEndian.PutUint32(temp, uint32(len(alg)))
	algorithmID := append(temp, alg...)
	other = append(other, algorithmID...)

	binary.BigEndian.PutUint32(temp, uint32(len(apu)))
	partyUInfo := append(temp, apu...)
	other = append(other, partyUInfo...)

	binary.BigEndian.PutUint32(temp, uint32(len(apv)))
	partyVInfo := append(temp, apv...)
	other = append(other, partyVInfo...)

	// SUppPubInfo is set to the keydatalen represented as a 32-bit big-endian integer
	binary.BigEndian.PutUint32(temp, uint32(kBit))
	other = append(other, temp...)

	// SuppPrivInfo is set to the empty octet sequence

	return other
}

func _NewECDHES(wrapper jweAlgFactory, alg []byte, kBit int, mJWK *jwk.JWK, header map[string]interface{}) (ec *ECDHES, err error) {
	if mJWK.Kty != jwk.EC {
		return nil, fmt.Errorf("unsupported algorithm: ECDH-ESx")
	}

	ec = &ECDHES{priv: mJWK, kBit: kBit, wrapper: wrapper}
	epk, ok := header["epk"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing header parameter: epk")
	}

	ec.pub, err = jwk.NewJWKPub(epk)
	if err != nil {
		return nil, err
	}

	if ec.pub.Kty != jwk.EC {
		return nil, fmt.Errorf("bad value for epk.kty, expected EC")
	}

	params, err := ParseBase64URLHeader(header, false, "apu", "apv")
	if err != nil {
		return nil, err
	}

	if alg == nil {
		alg = []byte(header["alg"].(string))
	}

	// get otherInfo
	ec.other = otherInfo(alg, params["apu"].Decoded, params["apv"].Decoded, kBit)

	return ec, nil
}

// Key Agreement with Elliptic Curve Diffie-Hellman Ephemeral Static
func NewECDHES(mJWK *jwk.JWK, header map[string]interface{}) (ec JWEAlg, err error) {
	enc := header["enc"].(string)
	return _NewECDHES(nil, []byte(enc), JWEEncKeyLength[enc], mJWK, header)
}

// ECDH-ES using Concat-KDF and key wrapped with A128KW
func NewECDHESA128KW(mJWK *jwk.JWK, header map[string]interface{}) (ec JWEAlg, err error) {
	return _NewECDHES(NewA128KW, nil, 128, mJWK, header)
}

// ECDH-ES using Concat-KDF and key wrapped with A192KW
func NewECDHESA192KW(mJWK *jwk.JWK, header map[string]interface{}) (ec JWEAlg, err error) {
	return _NewECDHES(NewA192KW, nil, 192, mJWK, header)
}

// ECDH-ES using Concat-KDF and key wrapped with A256KW
func NewECDHESA256KW(mJWK *jwk.JWK, header map[string]interface{}) (ec JWEAlg, err error) {
	return _NewECDHES(NewA256KW, nil, 256, mJWK, header)
}
