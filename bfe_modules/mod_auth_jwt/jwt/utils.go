package jwt

import (
	"encoding/json"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
)

type Base64URL = jwk.Base64URL

// base64url-encoded json
type Base64URLJson struct {
	Raw              string
	Decoded          map[string]interface{}
	DecodedBase64URL []byte
}

var (
	Base64URLDecode = jwk.Base64URLDecode
	NewBase64URL    = jwk.NewBase64URL
)

func NewBase64URLJson(raw string, strict bool) (b *Base64URLJson, err error) {
	// the parameter 'strict' tells whether json error should be report or not

	bDecoded, err := Base64URLDecode(raw)
	if err != nil {
		return nil, err
	}
	jMap := make(map[string]interface{})
	if err = json.Unmarshal(bDecoded, &jMap); err != nil {
		if strict {
			return nil, err
		}
		jMap = nil // in loose mode
	}
	return &Base64URLJson{raw, jMap, bDecoded}, nil
}
