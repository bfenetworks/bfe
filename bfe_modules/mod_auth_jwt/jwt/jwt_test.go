package jwt

import (
	"encoding/json"
	"fmt"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwa"
	"github.com/baidu/bfe/bfe_modules/mod_auth_jwt/jwk"
	"io/ioutil"
	"testing"
)

var config Config

func TestJWSValidate(t *testing.T) {
	for name := range jwa.JWSAlgSet {
		tokenPath := fmt.Sprintf("./../testdata/mod_auth_jwt/test_jws_%s.txt", name)
		secretPath := fmt.Sprintf("./../testdata/mod_auth_jwt/secret_test_jws_%s.key", name)
		token, _ := ioutil.ReadFile(tokenPath)
		secret, _ := ioutil.ReadFile(secretPath)
		keyMap := make(map[string]interface{})
		_ = json.Unmarshal(secret, &keyMap)
		config.Secret, _ = jwk.NewJWK(keyMap)
		mJWT, err := NewJWT(string(token), &config)
		if err != nil {
			t.Error(name, err)
		}
		if err := mJWT.Validate(); err != nil {
			t.Error(name, err)
		}
	}
}

func TestJWEValidate(t *testing.T) {
	for name := range jwa.JWEAlgSet {
		tokenPath := fmt.Sprintf("./../testdata/mod_auth_jwt/test_jwe_%s_A128GCM.txt", name)
		secretPath := fmt.Sprintf("./../testdata/mod_auth_jwt/secret_test_jwe_%s_A128GCM.key", name)
		token, _ := ioutil.ReadFile(tokenPath)
		secret, _ := ioutil.ReadFile(secretPath)
		keyMap := make(map[string]interface{})
		_ = json.Unmarshal(secret, &keyMap)
		config.Secret, _ = jwk.NewJWK(keyMap)
		mJWT, err := NewJWT(string(token), &config)
		if err != nil {
			t.Error(name, err)
		}
		if err := mJWT.Validate(); err != nil {
			t.Error(name, err)
		}
	}
}
