package jwa

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"io/ioutil"
	"strings"
	"testing"
)

func TestSignAlg(t *testing.T) {
	path := "./../testdata/mod_auth_jwt"
	for name, alg := range SignAlgs {
		secret, _ := ioutil.ReadFile(fmt.Sprintf("%s/secret_test_jws_%s.key", path, name))
		token, _ := ioutil.ReadFile(fmt.Sprintf("%s/test_jws_%s.txt", path, name))
		keyMap := make(map[string]interface{})
		_ = json.Unmarshal(secret, &keyMap)
		mJWK, _ := jwk.NewJWK(keyMap)
		handler, _ := alg(mJWK)
		tokens := strings.Split(string(token), ".")
		msg := []byte(strings.Join(tokens[:2], "."))
		sig, _ := base64.RawURLEncoding.DecodeString(tokens[2])
		_, _ = handler.Update(msg)
		if !handler.Verify(sig) {
			t.Errorf("algorithm %s test failed", name)
		}
	}
}
