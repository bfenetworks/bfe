package jwa

import "github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"

// algorithm calculating signature
type SignAlg interface {
	Update(msg []byte) (n int, err error) // update msg
	Sign() (sig []byte)                   // get signature
	Verify(sig []byte) bool               // verify signature
}

// algorithm use to encrypt & decrypt
type EncAlg interface {
	Encrypt(msg []byte) (cipher []byte)
	Decrypt(cipher []byte) (msg []byte)
}

type signAlgFactory func(*jwk.JWK) (SignAlg, error)

// exported algorithms
var (
	SignAlgs = map[string]signAlgFactory{
		"HS256": NewHS256,
		"HS384": NewHS384,
		"HS512": NewHS512,
	}
)
