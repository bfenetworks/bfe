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

func TestJWSAlg(t *testing.T) {
	path := "./../testdata/mod_auth_jwt"
	for name, alg := range JWSAlgSet {
		current := fmt.Sprintf("testing %s:", name)
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
			t.Error(current, "failed")
			continue
		}
		t.Log(current, "ok")
	}
}

// test algorithms not need header
func TestJWEAlg(t *testing.T) {
	path := "./../testdata/mod_auth_jwt"
	for name, alg := range JWEAlgSet {
		current := fmt.Sprintf("testing %s:", name)
		secret, _ := ioutil.ReadFile(fmt.Sprintf("%s/secret_test_jwe_%s_A128GCM.key", path, name))
		token, _ := ioutil.ReadFile(fmt.Sprintf("%s/test_jwe_%s_A128GCM.txt", path, name))
		keyMap := make(map[string]interface{})
		_ = json.Unmarshal(secret, &keyMap)
		mJWK, _ := jwk.NewJWK(keyMap)
		handler, _ := alg(mJWK, nil)
		eCek, _ := base64.RawURLEncoding.DecodeString(strings.Split(string(token), ".")[1])
		_, err := handler.Decrypt(eCek)
		if err != nil {
			t.Error(current, "failed")
			t.Log(err)
			continue
		}
		t.Log(current, "ok")
	}
}
