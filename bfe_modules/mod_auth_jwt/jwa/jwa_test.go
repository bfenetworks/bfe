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
	for name, alg := range JWSAlgSet {
		current := fmt.Sprintf("testing %s:", name)
		secret, _ := ioutil.ReadFile(fmt.Sprintf("%s/secret_test_jws_%s.key", relativePath, name))
		token, _ := ioutil.ReadFile(fmt.Sprintf("%s/test_jws_%s.txt", relativePath, name))
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

func TestJWEAlg(t *testing.T) {
	for name, alg := range JWEAlgSet {
		current := fmt.Sprintf("testing %s:", name)
		secret, _ := ioutil.ReadFile(fmt.Sprintf("%s/secret_test_jwe_%s_A128GCM.key", relativePath, name))
		token, _ := ioutil.ReadFile(fmt.Sprintf("%s/test_jwe_%s_A128GCM.txt", relativePath, name))
		tokens := strings.Split(string(token), ".")
		keyMap := make(map[string]interface{})
		header := make(map[string]interface{})
		headerStr, _ := base64.RawURLEncoding.DecodeString(tokens[0])
		_ = json.Unmarshal(secret, &keyMap)
		_ = json.Unmarshal(headerStr, &header)
		mJWK, _ := jwk.NewJWK(keyMap)
		handler, _ := alg(mJWK, header)
		eCek, _ := base64.RawURLEncoding.DecodeString(tokens[1])
		_, err := handler.Decrypt(eCek)
		if err != nil {
			t.Error(current, "failed")
			t.Log(err)
			continue
		}
		t.Log(current, "ok")
	}
}
