// defined item type for the key base64url-encoded and base64urlUInt-encoded

package jwk

import "math/big"

// base64url-encoded
type Base64URL struct {
	Raw     string
	Decoded []byte
}

// base64urlUInt-encoded
type Base64URLUint struct {
	Raw           string
	Decoded       *big.Int
	DecodedBase64 []byte
}

func NewBase64URL(raw string) (b *Base64URL, err error) {
	decoded, err := Base64URLDecode(raw)
	if err != nil {
		return nil, err
	}
	return &Base64URL{raw, decoded}, nil
}

func NewBase64URLUint(raw string) (b *Base64URLUint, err error) {
	oct, decoded, err := Base64URLUintDecode(raw)
	if err != nil {
		return nil, err
	}
	return &Base64URLUint{raw, decoded, oct}, nil
}
